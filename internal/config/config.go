package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// denyReadPath is the only key claudeignore owns in a settings file.
// Everything else the file contains is user- or Claude-Code-owned and is
// round tripped untouched.
var denyReadPath = []string{"sandbox", "filesystem", "denyRead"}

// Settings wraps a Claude Code settings JSON document. Reads and writes are
// surgical: only the keys claudeignore manages are modified, and every other
// key — including nested ones such as sandbox.network or
// sandbox.filesystem.allowWrite — is preserved with its original order and
// formatting.
type Settings struct {
	obj *jsonObject
}

// NewSettings returns an empty settings document.
func NewSettings() *Settings {
	return &Settings{obj: newJSONObject()}
}

// LoadSettings reads and parses a Claude Code settings JSON file.
func LoadSettings(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSettings(data)
}

// LoadOrCreateSettings reads a settings file, returning an empty document when
// the file does not exist yet. A file that exists but cannot be read or parsed
// is reported as an error: overwriting it would destroy the user's
// configuration, so callers must refuse to write rather than start fresh.
func LoadOrCreateSettings(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewSettings(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return NewSettings(), nil
	}
	o, err := parseJSONObject(data)
	if err != nil {
		return nil, fmt.Errorf("%s contains invalid JSON (%w) — claudeignore will not overwrite it; fix the file, then run 'claudeignore sync'", path, err)
	}
	return &Settings{obj: o}, nil
}

// ParseSettings parses raw JSON into Settings, preserving every key.
func ParseSettings(data []byte) (*Settings, error) {
	o, err := parseJSONObject(data)
	if err != nil {
		return nil, fmt.Errorf("invalid settings JSON: %w", err)
	}
	return &Settings{obj: o}, nil
}

// MarshalJSON produces compact JSON with keys in their original order.
func (s *Settings) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("{}"), nil
	}
	return s.obj.MarshalJSON()
}

// Get returns the decoded value of a top-level key.
func (s *Settings) Get(key string) (interface{}, bool) {
	if s == nil || s.obj == nil {
		return nil, false
	}
	raw, ok := s.obj.get(key)
	if !ok {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return v, true
}

// Set writes a top-level key, leaving the position of the other keys intact.
func (s *Settings) Set(key string, value interface{}) error {
	return s.SetPath(value, key)
}

// SetPath writes a value at a nested key path, creating intermediate objects as
// needed and preserving sibling keys at every level.
func (s *Settings) SetPath(value interface{}, keys ...string) error {
	if s.obj == nil {
		s.obj = newJSONObject()
	}
	raw, err := encodeJSON(value)
	if err != nil {
		return err
	}
	return s.obj.setPath(raw, keys...)
}

// GetDenyList extracts the sandbox.filesystem.denyRead list from settings.
func (s *Settings) GetDenyList() []string {
	if s == nil || s.obj == nil {
		return nil
	}
	raw, ok := s.obj.getPath(denyReadPath...)
	if !ok {
		return nil
	}
	var deny []string
	if err := json.Unmarshal(raw, &deny); err != nil {
		return nil
	}
	return deny
}

// SetDenyList replaces the sandbox.filesystem.denyRead list. Sibling keys under
// sandbox and sandbox.filesystem are left alone.
func (s *Settings) SetDenyList(deny []string) error {
	if deny == nil {
		deny = []string{}
	}
	return s.SetPath(deny, denyReadPath...)
}

// SaveSettings writes settings to a file, preserving every key it did not
// manage. The write is atomic so an interrupted run cannot truncate the file.
func SaveSettings(path string, s *Settings) error {
	out, err := s.MarshalJSON()
	if err != nil {
		return err
	}
	pretty, err := formatJSON(out)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(pretty, '\n'), 0600)
}

// UpdateSettingsFile reads a settings file, updates denyRead, and writes it
// back. Any other setting present in the file is preserved; an unparseable file
// is reported rather than overwritten.
func UpdateSettingsFile(settingsPath string, deny []string) error {
	s, err := LoadOrCreateSettings(settingsPath)
	if err != nil {
		return err
	}
	if err := s.SetDenyList(deny); err != nil {
		return err
	}
	return SaveSettings(settingsPath, s)
}

// WriteFileAtomic writes data to a temporary file in the target directory and
// renames it into place, so readers never observe a half-written file. The
// permissions of an existing file are kept.
//
// When the directory does not allow creating the temporary file but the target
// itself is writable, it falls back to a direct write: losing atomicity beats
// refusing to sync at all.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".claudeignore-*.tmp")
	if err != nil {
		return os.WriteFile(path, data, perm) //nolint:gosec // perm mirrors the existing file, or the caller's default
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// --- File helpers ---

// ReadLines reads non-empty, non-comment lines from a file.
func ReadLines(path string) []string {
	data, err := os.ReadFile(path)
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
	requiredEntries := []string{"state.json", "hook-*.log"}

	var existing []string
	if data, err := os.ReadFile(gitignorePath); err == nil {		for _, line := range strings.Split(string(data), "\n") {
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
		if err := os.WriteFile(gitignorePath, []byte(content), 0600); err != nil { //nolint:gosec // path constructed via filepath.Join
			return fmt.Errorf("cannot write %s: %w", gitignorePath, err)
		}
	}
	return nil
}
