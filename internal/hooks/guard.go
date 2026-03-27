package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/claudeous/claudeignore/internal/config"
)

// Guard is the PreToolUse hook handler.
// It reads JSON from stdin, extracts the file path, and checks against denyRead.
// Returns (blocked, reason, error). If blocked is true, the caller should exit 2.
func Guard(root string) (blocked bool, reason string, err error) {
	// Read deny list from settings.local.json
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	settings, err := config.LoadSettings(settingsPath)
	if err != nil {
		return false, "", nil // can't read settings, allow
	}

	denyList := settings.GetDenyList()
	if len(denyList) == 0 {
		return false, "", nil
	}

	// Read hook input from stdin
	input, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return false, "", nil // can't read stdin, allow
	}

	var hookInput struct {
		ToolName  string                 `json:"tool_name"`
		ToolInput map[string]interface{} `json:"tool_input"`
	}
	if json.Unmarshal(input, &hookInput) != nil {
		return false, "", nil
	}

	if hookInput.ToolInput == nil {
		return false, "", nil
	}

	// Extract file path from tool_input
	var targetPath string
	if fp, ok := hookInput.ToolInput["file_path"].(string); ok {
		targetPath = fp
	} else if p, ok := hookInput.ToolInput["path"].(string); ok {
		targetPath = p
	} else if _, ok := hookInput.ToolInput["pattern"].(string); ok {
		// Glob pattern, not a path — skip
		return false, "", nil
	}

	if targetPath == "" {
		return false, "", nil
	}

	return CheckPathBlocked(root, targetPath, denyList)
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
