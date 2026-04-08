package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/claudeous/claudeignore/internal/config"
)

// GuardResult holds the outcome of the guard check.
type GuardResult struct {
	Blocked      bool
	Reason       string
	UpdatedInput json.RawMessage // non-nil when we need to output updatedInput on stdout
}

// Guard is the PreToolUse hook handler.
// It reads JSON from stdin, extracts the file path, and checks against denyRead.
func Guard(root string) (*GuardResult, error) {
	// Read deny list from settings.local.json
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	settings, err := config.LoadSettings(settingsPath)
	if err != nil {
		return &GuardResult{}, nil // can't read settings, allow
	}

	denyList := settings.GetDenyList()
	if len(denyList) == 0 {
		return &GuardResult{}, nil
	}

	// Read hook input from stdin.
	// Uses io.ReadAll(os.Stdin) instead of os.ReadFile("/dev/stdin") for
	// cross-platform compatibility — /dev/stdin does not exist on Windows,
	// which caused the guard to silently fail-open with no protection.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return &GuardResult{}, nil // can't read stdin, allow
	}

	var hookInput struct {
		ToolName  string                 `json:"tool_name"`
		ToolInput map[string]interface{} `json:"tool_input"`
	}
	if json.Unmarshal(input, &hookInput) != nil {
		return &GuardResult{}, nil
	}

	if hookInput.ToolInput == nil {
		return &GuardResult{}, nil
	}

	// Handle Grep specially for broad searches
	if hookInput.ToolName == "Grep" {
		return guardGrep(root, hookInput.ToolInput, denyList)
	}

	// Extract file path from tool_input (Read, Write, Edit, NotebookEdit, etc.)
	var targetPath string
	if fp, ok := hookInput.ToolInput["file_path"].(string); ok {
		targetPath = fp
	} else if p, ok := hookInput.ToolInput["path"].(string); ok {
		targetPath = p
	}

	if targetPath == "" {
		return &GuardResult{}, nil
	}

	blocked, reason, err := CheckPathBlocked(root, targetPath, denyList)
	if err != nil {
		return &GuardResult{}, nil
	}
	return &GuardResult{Blocked: blocked, Reason: reason}, nil
}

// guardGrep handles the Grep tool with three scenarios:
// A) No path, no glob: inject exclusion glob via updatedInput
// B) No path, has glob: check intersection with deny list, block if any match
// C) Has path that is parent of denied entries: inject exclusion glob via updatedInput
// Otherwise: existing path-based check applies.
func guardGrep(root string, toolInput map[string]interface{}, denyList []string) (*GuardResult, error) {
	targetPath, hasPath := toolInput["path"].(string)
	existingGlob, hasGlob := toolInput["glob"].(string)

	// Determine the effective search base for glob computation.
	// If Grep has an explicit path, use it. Otherwise fall back to cwd
	// (which is where ripgrep will search).
	searchBase := root
	if hasPath && targetPath != "" {
		abs, err := filepath.Abs(targetPath)
		if err == nil {
			searchBase = abs
		}
	} else {
		cwd, err := os.Getwd()
		if err == nil {
			searchBase = cwd
		}
	}

	// If there's a specific path, check if it's directly blocked
	if hasPath && targetPath != "" {
		blocked, reason, err := CheckPathBlocked(root, targetPath, denyList)
		if err != nil {
			return &GuardResult{}, nil // fail open
		}
		if blocked {
			return &GuardResult{Blocked: true, Reason: reason}, nil
		}

		// Check if path is a parent of any denied entries
		if isParentOfDenied(root, targetPath, denyList) {
			updatedJSON, err := buildUpdatedInputJSON(root, searchBase, toolInput, denyList)
			if err != nil {
				return &GuardResult{}, nil // fail open
			}
			return &GuardResult{UpdatedInput: updatedJSON}, nil
		}

		// Path is unrelated to deny list, allow as-is
		return &GuardResult{}, nil
	}

	// No path (or empty path)
	if hasGlob && existingGlob != "" {
		if globIntersectsDenyList(root, existingGlob, denyList) {
			reason := "[claudeignore] Access denied: Grep glob pattern matches denied files"
			return &GuardResult{Blocked: true, Reason: reason}, nil
		}
		return &GuardResult{}, nil
	}

	// Scenario A: no path, no glob → inject exclusion glob
	updatedJSON, err := buildUpdatedInputJSON(root, searchBase, toolInput, denyList)
	if err != nil {
		return &GuardResult{}, nil // fail open
	}
	return &GuardResult{UpdatedInput: updatedJSON}, nil
}

// isParentOfDenied checks whether the given path is a parent directory
// of any entry in the deny list.
func isParentOfDenied(root, targetPath string, denyList []string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	relPath, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	if strings.HasPrefix(relPath, "..") {
		return false
	}

	normalizedPath := config.Normalize(relPath)

	// Special case: "." means repo root, which is parent of everything
	if normalizedPath == "." {
		return len(denyList) > 0
	}

	for _, deny := range denyList {
		denyNorm := config.Normalize(deny)
		if strings.HasPrefix(denyNorm, normalizedPath+"/") {
			return true
		}
	}
	return false
}

// BuildExclusionGlob creates a ripgrep exclusion glob pattern from the deny list.
// Patterns are computed relative to searchBase so they work when ripgrep runs
// from a subdirectory of the repo root.
// File entries (containing a dot in basename) stay as-is, directory-like entries get /** appended.
// Entries outside searchBase are skipped (not in the search tree).
func BuildExclusionGlob(root, searchBase string, denyList []string) string {
	if len(denyList) == 0 {
		return ""
	}

	parts := make([]string, 0, len(denyList))
	for _, deny := range denyList {
		norm := config.Normalize(deny)
		if norm == "" {
			continue
		}

		// Compute absolute path of the denied entry
		absDeny := filepath.Join(root, norm)

		// Compute path relative to the search base
		relPath, err := filepath.Rel(searchBase, absDeny)
		if err != nil {
			continue
		}

		// Skip entries outside the search base (they start with "..")
		if strings.HasPrefix(relPath, "..") {
			continue
		}

		// Treat entries with a dot in the last path component as files,
		// everything else as directories.
		base := filepath.Base(relPath)
		if strings.Contains(base, ".") {
			parts = append(parts, relPath)
		} else {
			parts = append(parts, relPath+"/**")
		}
	}

	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		return "!" + parts[0]
	}

	return "!{" + strings.Join(parts, ",") + "}"
}

// buildUpdatedInputJSON constructs the full hookSpecificOutput JSON with updatedInput.
func buildUpdatedInputJSON(root, searchBase string, toolInput map[string]interface{}, denyList []string) (json.RawMessage, error) {
	exclusionGlob := BuildExclusionGlob(root, searchBase, denyList)
	if exclusionGlob == "" {
		return nil, fmt.Errorf("empty exclusion glob")
	}

	// Copy all original tool input fields into updatedInput
	updatedInput := make(map[string]interface{}, len(toolInput)+1)
	for k, v := range toolInput {
		updatedInput[k] = v
	}
	// Override/set the glob with our exclusion pattern
	updatedInput["glob"] = exclusionGlob

	output := map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
			"hookEventName":      "PreToolUse",
			"permissionDecision": "allow",
			"updatedInput":       updatedInput,
		},
	}

	return json.Marshal(output)
}

// globIntersectsDenyList checks if a glob pattern could match any denied files.
// It resolves the glob against the repo root and checks each match against the deny list.
func globIntersectsDenyList(root string, globPattern string, denyList []string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false // fail open
	}

	// Try to resolve the glob pattern relative to the repo root
	var matches []string
	if filepath.IsAbs(globPattern) {
		matches, err = filepath.Glob(globPattern)
	} else {
		matches, err = filepath.Glob(filepath.Join(absRoot, globPattern))
	}
	if err != nil || len(matches) == 0 {
		// If glob resolution fails or finds nothing, check via pattern matching
		return globPatternMatchesDenyList(globPattern, denyList)
	}

	// Check if any matched file is in the deny list
	for _, match := range matches {
		relPath, err := filepath.Rel(absRoot, match)
		if err != nil {
			continue
		}
		if strings.HasPrefix(relPath, "..") {
			continue
		}
		for _, deny := range denyList {
			denyNorm := config.Normalize(deny)
			if relPath == denyNorm || strings.HasPrefix(relPath, denyNorm+"/") {
				return true
			}
		}
	}

	return false
}

// globPatternMatchesDenyList performs a heuristic check for whether a glob pattern
// could match denied entries when filepath.Glob cannot resolve it.
func globPatternMatchesDenyList(globPattern string, denyList []string) bool {
	for _, deny := range denyList {
		denyNorm := config.Normalize(deny)
		matched, err := filepath.Match(globPattern, denyNorm)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
		// Check if the glob targets a denied directory
		// e.g., glob "secrets/*" and deny "secrets"
		if strings.HasSuffix(globPattern, "/*") || strings.HasSuffix(globPattern, "/**") {
			dir := strings.TrimRight(globPattern, "/*")
			if denyNorm == dir || strings.HasPrefix(denyNorm, dir+"/") {
				return true
			}
		}
		// Check if denied entry is a directory and glob matches within it
		if strings.HasPrefix(globPattern, denyNorm+"/") || globPattern == denyNorm {
			return true
		}
	}
	return false
}

// CheckPathBlocked tests whether a given path is blocked by the deny list.
func CheckPathBlocked(root, targetPath string, denyList []string) (blocked bool, reason string, err error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, "", nil
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false, "", nil
	}

	relPath, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, "", nil
	}

	// Path traversal protection: if the resolved path escapes the repo root, skip
	if strings.HasPrefix(relPath, "..") {
		return false, "", nil
	}

	// Check if path matches any deny entry (prefix matching like sandbox)
	for _, deny := range denyList {
		denyNorm := config.Normalize(deny)
		if relPath == denyNorm || strings.HasPrefix(relPath, denyNorm+"/") {
			reason := fmt.Sprintf("[claudeignore] Access denied: %s is in denyRead list", relPath)
			return true, reason, nil
		}
	}

	return false, "", nil
}

// GuardDenyResponse returns the JSON to write to stderr when blocking.
func GuardDenyResponse(reason string) []byte {
	result := map[string]string{
		"decision": "deny",
		"reason":   reason,
	}
	out, _ := json.Marshal(result)
	return out
}
