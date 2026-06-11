package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIgnoredOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "no ignored files",
			input:    "M  main.go\n?? newfile.txt\n",
			expected: nil,
		},
		{
			name:     "single ignored file",
			input:    "!! .env\n",
			expected: []string{".env"},
		},
		{
			name:     "multiple ignored files",
			input:    "!! .env\n!! node_modules/\n!! dist/\n",
			expected: []string{".env", "node_modules", "dist"},
		},
		{
			name:     "trailing slash stripped",
			input:    "!! vendor/\n",
			expected: []string{"vendor"},
		},
		{
			name:     "mixed porcelain output",
			input:    " M main.go\n?? todo.txt\n!! .env\n!! secret/\nA  new.go\n",
			expected: []string{".env", "secret"},
		},
		{
			name:     "empty lines ignored",
			input:    "\n!! .env\n\n!! .secret\n\n",
			expected: []string{".env", ".secret"},
		},
		{
			name:     "whitespace-only lines ignored",
			input:    "   \n!! .env\n  \n",
			expected: []string{".env"},
		},
		{
			name:     "nested path",
			input:    "!! config/secrets/prod.env\n",
			expected: []string{"config/secrets/prod.env"},
		},
		{
			name:     "path with spaces",
			input:    "!! my folder/secret file.txt\n",
			expected: []string{"my folder/secret file.txt"},
		},
		{
			name:     "OS noise files filtered",
			input:    "!! .DS_Store\n!! Thumbs.db\n!! ehthumbs.db\n!! desktop.ini\n",
			expected: nil,
		},
		{
			name:     "OS noise files filtered in subdirectories",
			input:    "!! docs/.DS_Store\n!! assets/img/Thumbs.db\n",
			expected: nil,
		},
		{
			name:     "AppleDouble resource forks filtered",
			input:    "!! ._secret.txt\n!! docs/._notes.md\n",
			expected: nil,
		},
		{
			name:     "noise filtered but real paths kept",
			input:    "!! .DS_Store\n!! .env\n!! docs/.DS_Store\n!! secret/\n",
			expected: []string{".env", "secret"},
		},
		{
			name:     "noise basename only matched, not directories containing it",
			input:    "!! DS_Store-tools/config.env\n",
			expected: []string{"DS_Store-tools/config.env"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseIgnoredOutput([]byte(tt.input))

			if len(result) != len(tt.expected) {
				t.Fatalf("got %d paths, want %d\ngot:  %v\nwant: %v", len(result), len(tt.expected), result, tt.expected)
			}
			for i, got := range result {
				if got != tt.expected[i] {
					t.Errorf("path[%d] = %q, want %q", i, got, tt.expected[i])
				}
			}
		})
	}
}

// TestAllIgnoredPaths_ClaudeIgnoreBlocksTrackedFile reproduces the bug where a
// file listed in .claude.ignore is silently skipped if it is tracked by git.
// .claude.ignore expresses "extra deny" intent: it must block matching files
// regardless of their tracked state.
func TestAllIgnoredPaths_ClaudeIgnoreBlocksTrackedFile(t *testing.T) {
	root := setupGitRepo(t, "tracked-deny")

	// Track a config file in git
	if err := os.WriteFile(filepath.Join(root, "config.yml"), []byte("k: v\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "config.yml")
	run(t, root, "git", "commit", "-m", "add config")

	// User wants to block config.yml from Claude via .claude.ignore
	if err := os.WriteFile(filepath.Join(root, ".claude.ignore"), []byte("config.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := AllIgnoredPaths(root)
	if err != nil {
		t.Fatalf("AllIgnoredPaths: %v", err)
	}

	found := false
	for _, p := range paths {
		if p == "config.yml" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("config.yml is tracked AND listed in .claude.ignore — must appear in AllIgnoredPaths; got %v", paths)
	}
}
