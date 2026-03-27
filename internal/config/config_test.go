package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"no slashes", "file.txt", "file.txt"},
		{"trailing slash", "dir/", "dir"},
		{"leading slash", "/dir", "dir"},
		{"both slashes", "/dir/", "dir"},
		{"nested path", "a/b/c", "a/b/c"},
		{"nested with trailing", "a/b/c/", "a/b/c"},
		{"nested with leading", "/a/b/c", "a/b/c"},
		{"dot file", ".env", ".env"},
		{"double slash not stripped", "a//b", "a//b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewPathSet_and_PathSetContains(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		lookup   string
		expected bool
	}{
		{"exact match", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b"}, "c", false},
		{"with trailing slash", []string{"dir/"}, "dir", true},
		{"lookup with trailing slash", []string{"dir"}, "dir/", true},
		{"empty set", nil, "anything", false},
		{"empty lookup", []string{"a"}, "", false},
		{"with leading slash", []string{"/dir"}, "dir", true},
		{"dot file", []string{".env"}, ".env", true},
		{"nested path", []string{"config/secrets"}, "config/secrets", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := NewPathSet(tt.paths)
			got := PathSetContains(set, tt.lookup)
			if got != tt.expected {
				t.Errorf("PathSetContains(%v, %q) = %v, want %v", tt.paths, tt.lookup, got, tt.expected)
			}
		})
	}
}

func TestPathMatchesSet(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		lookup   string
		expected bool
	}{
		{"exact match", []string{".env"}, ".env", true},
		{"not found", []string{".env"}, "other.txt", false},
		{"dir prefix matches nested file", []string{"pdf"}, "pdf/Client1.pdf", true},
		{"dir prefix with trailing slash", []string{"pdf/"}, "pdf/Client1.pdf", true},
		{"dir prefix matches deeply nested", []string{"vendor"}, "vendor/github.com/pkg/a.go", true},
		{"partial dir name does not match", []string{"pdf"}, "pdf2/file.txt", false},
		{"nested dir prefix", []string{"config/secrets"}, "config/secrets/prod.env", true},
		{"parent dir only matches children", []string{"config"}, "config/secrets/prod.env", true},
		{"empty set", nil, "anything", false},
		{"exact match file in dir", []string{"pdf/specific.pdf"}, "pdf/specific.pdf", true},
		{"file does not prefix-match sibling", []string{"pdf/a.pdf"}, "pdf/b.pdf", false},
		{"root file not matched by similar dir", []string{"src"}, "src-old/file.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := NewPathSet(tt.paths)
			got := PathMatchesSet(set, tt.lookup)
			if got != tt.expected {
				t.Errorf("PathMatchesSet(%v, %q) = %v, want %v", tt.paths, tt.lookup, got, tt.expected)
			}
		})
	}
}

func TestReadLines(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "normal lines",
			content:  "a\nb\nc\n",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "comments and blanks",
			content:  "# comment\n\na\n\n# another\nb\n",
			expected: []string{"a", "b"},
		},
		{
			name:     "whitespace trimmed",
			content:  "  a  \n  b  \n",
			expected: []string{"a", "b"},
		},
		{
			name:     "empty file",
			content:  "",
			expected: nil,
		},
		{
			name:     "only comments",
			content:  "# comment\n# another\n",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".txt")
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}

			got := ReadLines(path)

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d lines, want %d\ngot:  %v\nwant: %v", len(got), len(tt.expected), got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("line[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}

	// Test missing file
	t.Run("missing file", func(t *testing.T) {
		got := ReadLines(filepath.Join(dir, "nonexistent"))
		if got != nil {
			t.Errorf("expected nil for missing file, got %v", got)
		}
	})
}

func TestWriteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	err := WriteLines(path, "# header", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("WriteLines error: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test file path
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	expected := "# header\na\nb\nc\n"
	if string(data) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), expected)
	}
}

func TestSettings_ParseAndMarshal(t *testing.T) {
	t.Run("parse settings with denyRead", func(t *testing.T) {
		input := `{
  "sandbox": {
    "filesystem": {
      "denyRead": [".env", "secrets/"]
    }
  },
  "someOtherKey": "preserved"
}`
		s, err := ParseSettings([]byte(input))
		if err != nil {
			t.Fatalf("ParseSettings error: %v", err)
		}

		deny := s.GetDenyList()
		if len(deny) != 2 || deny[0] != ".env" || deny[1] != "secrets/" {
			t.Errorf("GetDenyList() = %v, want [.env secrets/]", deny)
		}

		// Verify extra keys preserved
		if _, ok := s.Extra["someOtherKey"]; !ok {
			t.Error("extra key 'someOtherKey' was not preserved")
		}
	})

	t.Run("empty settings", func(t *testing.T) {
		s, err := ParseSettings([]byte(`{}`))
		if err != nil {
			t.Fatalf("ParseSettings error: %v", err)
		}
		if deny := s.GetDenyList(); deny != nil {
			t.Errorf("expected nil deny list, got %v", deny)
		}
	})

	t.Run("set deny list on empty settings", func(t *testing.T) {
		s := &Settings{Extra: make(map[string]interface{})}
		s.SetDenyList([]string{".env", "dist"})
		deny := s.GetDenyList()
		if len(deny) != 2 {
			t.Fatalf("expected 2 items, got %d", len(deny))
		}
	})

	t.Run("roundtrip preserves structure", func(t *testing.T) {
		input := `{"sandbox":{"filesystem":{"denyRead":[".env"]}},"custom":"value"}`
		s, _ := ParseSettings([]byte(input))
		out, err := s.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON error: %v", err)
		}

		// Parse back
		var m map[string]interface{}
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatal(err)
		}
		if m["custom"] != "value" {
			t.Error("custom key not preserved in roundtrip")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := ParseSettings([]byte(`not json`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestUpdateSettingsFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("creates new file", func(t *testing.T) {
		path := filepath.Join(dir, "new.json")
		err := UpdateSettingsFile(path, []string{".env", "secrets"})
		if err != nil {
			t.Fatalf("UpdateSettingsFile error: %v", err)
		}

		s, err := LoadSettings(path)
		if err != nil {
			t.Fatalf("LoadSettings error: %v", err)
		}
		deny := s.GetDenyList()
		if len(deny) != 2 {
			t.Errorf("expected 2 deny entries, got %d", len(deny))
		}
	})

	t.Run("preserves existing keys", func(t *testing.T) {
		path := filepath.Join(dir, "existing.json")
		if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]}}`), 0600); err != nil {
			t.Fatal(err)
		}

		err := UpdateSettingsFile(path, []string{".env"})
		if err != nil {
			t.Fatalf("UpdateSettingsFile error: %v", err)
		}

		data, err2 := os.ReadFile(path) //nolint:gosec // test file path
		if err2 != nil {
			t.Fatal(err2)
		}
		var m map[string]interface{}
		if err3 := json.Unmarshal(data, &m); err3 != nil {
			t.Fatal(err3)
		}
		if m["permissions"] == nil {
			t.Error("existing 'permissions' key was not preserved")
		}
	})
}

func TestEnsureClaudeGitignore(t *testing.T) {
	t.Run("creates gitignore from scratch", func(t *testing.T) {
		dir := t.TempDir()
		err := EnsureClaudeGitignore(dir)
		if err != nil {
			t.Fatalf("EnsureClaudeGitignore error: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, ".claude", "claudeignore", ".gitignore")) //nolint:gosec // test path
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}
		if !contains(string(data), "state.json") {
			t.Error("missing state.json entry")
		}
	})

	t.Run("does not duplicate entries", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureClaudeGitignore(dir); err != nil {
			t.Fatal(err)
		}
		if err := EnsureClaudeGitignore(dir); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(dir, ".claude", "claudeignore", ".gitignore")
		data, _ := os.ReadFile(path) //nolint:gosec // test path
		lines := 0
		for _, line := range splitLines(string(data)) {
			if line == "state.json" {
				lines++
			}
		}
		if lines != 1 {
			t.Errorf("state.json appears %d times, want 1", lines)
		}
	})

	t.Run("preserves existing entries", func(t *testing.T) {
		dir := t.TempDir()
		ciDir := filepath.Join(dir, ".claude", "claudeignore")
		if err := os.MkdirAll(ciDir, 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ciDir, ".gitignore"), []byte("custom-entry\n"), 0600); err != nil {
			t.Fatal(err)
		}

		if err := EnsureClaudeGitignore(dir); err != nil {
			t.Fatal(err)
		}

		data, _ := os.ReadFile(filepath.Join(ciDir, ".gitignore"))
		content := string(data)
		if !contains(content, "custom-entry") {
			t.Error("existing entry 'custom-entry' was removed")
		}
	})
}

func contains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && stringContains(haystack, needle)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
