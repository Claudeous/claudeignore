package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings represents the Claude Code settings.local.json structure.
type Settings struct {
	Sandbox *SandboxSettings       `json:"sandbox,omitempty"`
	Hooks   map[string]interface{} `json:"hooks,omitempty"`
	Extra   map[string]interface{} `json:"-"` // captures other top-level keys
}

type SandboxSettings struct {
	Filesystem *FilesystemSettings `json:"filesystem,omitempty"`
}

type FilesystemSettings struct {
	DenyRead []string `json:"denyRead,omitempty"`
}

// LoadSettings reads and parses a Claude Code settings JSON file.
func LoadSettings(path string) (*Settings, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths are from known config locations
	if err != nil {
		return nil, err
	}
	return ParseSettings(data)
}

// ParseSettings parses raw JSON into Settings, preserving unknown keys.
func ParseSettings(data []byte) (*Settings, error) {
	// First, unmarshal into a raw map to capture all keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid settings JSON: %w", err)
	}

	s := &Settings{
		Extra: make(map[string]interface{}),
	}

	// Parse known keys
	if v, ok := raw["sandbox"]; ok {
		s.Sandbox = &SandboxSettings{}
		if err := json.Unmarshal(v, s.Sandbox); err != nil {
			s.Sandbox = nil
		}
		delete(raw, "sandbox")
	}
	if v, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(v, &s.Hooks); err != nil {
			s.Hooks = nil
		}
		delete(raw, "hooks")
	}

	// Preserve unknown keys
	for k, v := range raw {
		var val interface{}
		if err := json.Unmarshal(v, &val); err != nil {
			continue
		}
		s.Extra[k] = val
	}

	return s, nil
}

// MarshalJSON produces JSON that includes both known and extra keys.
func (s *Settings) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})

	// Copy extra keys first
	for k, v := range s.Extra {
		m[k] = v
	}

	// Known keys override
	if s.Sandbox != nil {
		m["sandbox"] = s.Sandbox
	}
	if s.Hooks != nil {
		m["hooks"] = s.Hooks
	}

	return json.MarshalIndent(m, "", "  ")
}

// GetDenyList extracts the denyRead list from settings.
func (s *Settings) GetDenyList() []string {
	if s == nil || s.Sandbox == nil || s.Sandbox.Filesystem == nil {
		return nil
	}
	return s.Sandbox.Filesystem.DenyRead
}

// SetDenyList updates the denyRead list in settings.
func (s *Settings) SetDenyList(deny []string) {
	if s.Sandbox == nil {
		s.Sandbox = &SandboxSettings{}
	}
	if s.Sandbox.Filesystem == nil {
		s.Sandbox.Filesystem = &FilesystemSettings{}
	}
	s.Sandbox.Filesystem.DenyRead = deny
}

// SaveSettings writes settings to a file, preserving unknown keys.
func SaveSettings(path string, s *Settings) error {
	out, err := s.MarshalJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
}

// UpdateSettingsFile reads a settings file, updates denyRead, and writes it back.
func UpdateSettingsFile(settingsPath string, deny []string) error {
	s, err := LoadSettings(settingsPath)
	if err != nil {
		// File doesn't exist or is invalid — start fresh
		s = &Settings{Extra: make(map[string]interface{})}
	}
	s.SetDenyList(deny)
	return SaveSettings(settingsPath, s)
}

// --- File helpers ---

// ReadLines reads non-empty, non-comment lines from a file.
func ReadLines(path string) []string {
	data, err := os.ReadFile(path) //nolint:gosec // paths are from known config locations
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// WriteLines writes a header and lines to a file.
func WriteLines(path string, header string, lines []string) error {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}

// Normalize strips leading and trailing slashes from a path.
func Normalize(s string) string {
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimPrefix(s, "/")
	return s
}

// NewPathSet builds a set of normalized paths for O(1) lookup.
func NewPathSet(paths []string) map[string]struct{} {
	s := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		s[Normalize(p)] = struct{}{}
	}
	return s
}

// PathSetContains checks if a normalized path is in the set (exact match only).
func PathSetContains(set map[string]struct{}, item string) bool {
	_, ok := set[Normalize(item)]
	return ok
}

// PathMatchesSet checks if a path is matched by the set, supporting both:
// - exact match: "file.txt" matches "file.txt"
// - prefix match: "pdf/Client1.pdf" matches if "pdf" is in the set
//
// This is used for .claude.unignore where directory entries (e.g. "pdf/")
// should match all files under that directory.
func PathMatchesSet(set map[string]struct{}, item string) bool {
	norm := Normalize(item)

	// Exact match
	if _, ok := set[norm]; ok {
		return true
	}

	// Check if any ancestor directory is in the set
	for i := 0; i < len(norm); i++ {
		if norm[i] == '/' {
			prefix := norm[:i]
			if _, ok := set[prefix]; ok {
				return true
			}
		}
	}

	return false
}

// EnsureClaudeGitignore ensures .claude/claudeignore/.gitignore exists
// with state.json ignored (local-only file).
func EnsureClaudeGitignore(root string) error {
	dir := filepath.Join(root, ".claude", "claudeignore")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	requiredEntries := []string{"state.json"}

	var existing []string
	if data, err := os.ReadFile(gitignorePath); err == nil { //nolint:gosec // known path
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				existing = append(existing, line)
			}
		}
	}

	changed := false
	existingSet := make(map[string]struct{}, len(existing))
	for _, line := range existing {
		existingSet[line] = struct{}{}
	}
	for _, entry := range requiredEntries {
		if _, ok := existingSet[entry]; !ok {
			existing = append(existing, entry)
			changed = true
		}
	}

	if changed {
		content := strings.Join(existing, "\n") + "\n"
		if err := os.WriteFile(gitignorePath, []byte(content), 0600); err != nil {
			return fmt.Errorf("cannot write %s: %w", gitignorePath, err)
		}
	}
	return nil
}
