package git

import (
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

func TestCollapsePaths(t *testing.T) {
	t.Run("no collapse under threshold", func(t *testing.T) {
		paths := []string{"config/a.yml", "config/b.yml", ".env"}
		result := CollapsePaths(paths)

		if len(result) != 3 {
			t.Fatalf("expected 3 entries, got %d: %+v", len(result), result)
		}
		for _, r := range result {
			if r.IsDir() {
				t.Errorf("expected no collapsed dirs, got %s with count %d", r.Path, r.Count)
			}
		}
	})

	t.Run("collapses large directory", func(t *testing.T) {
		paths := []string{
			".env",
			"pdf/a.pdf", "pdf/b.pdf", "pdf/c.pdf",
			"pdf/d.pdf", "pdf/e.pdf", "pdf/f.pdf",
		}
		result := CollapsePaths(paths)

		// Should be: .env + pdf/ (collapsed)
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d: %+v", len(result), result)
		}

		// Find the collapsed dir
		var found bool
		for _, r := range result {
			if r.Path == "pdf" && r.IsDir() {
				found = true
				if r.Count != 6 {
					t.Errorf("expected count 6, got %d", r.Count)
				}
				if len(r.Children) != 6 {
					t.Errorf("expected 6 children, got %d", len(r.Children))
				}
			}
		}
		if !found {
			t.Error("collapsed 'pdf' entry not found")
		}
	})

	t.Run("root files preserved", func(t *testing.T) {
		paths := []string{".env", ".secret", "credentials.json"}
		result := CollapsePaths(paths)

		if len(result) != 3 {
			t.Fatalf("expected 3, got %d", len(result))
		}
		for _, r := range result {
			if r.IsDir() {
				t.Error("root files should not be collapsed")
			}
		}
	})

	t.Run("multiple directories mixed", func(t *testing.T) {
		paths := []string{
			".env",
			"pdf/1.pdf", "pdf/2.pdf", "pdf/3.pdf", "pdf/4.pdf", "pdf/5.pdf", "pdf/6.pdf",
			"config/a.yml", "config/b.yml",
		}
		result := CollapsePaths(paths)

		// .env + pdf/ (collapsed) + config/a.yml + config/b.yml = 4
		if len(result) != 4 {
			t.Fatalf("expected 4, got %d: %+v", len(result), result)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result := CollapsePaths(nil)
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("nested directories collapse at first level", func(t *testing.T) {
		paths := []string{
			"vendor/github.com/pkg1/a.go",
			"vendor/github.com/pkg1/b.go",
			"vendor/github.com/pkg2/a.go",
			"vendor/github.com/pkg2/b.go",
			"vendor/github.com/pkg3/a.go",
			"vendor/golang.org/x/net/a.go",
		}
		result := CollapsePaths(paths)

		// All under vendor/ (6 files > threshold) → collapsed as "vendor"
		if len(result) != 1 {
			t.Fatalf("expected 1 collapsed entry, got %d: %+v", len(result), result)
		}
		if result[0].Path != "vendor" || result[0].Count != 6 {
			t.Errorf("expected vendor(6), got %s(%d)", result[0].Path, result[0].Count)
		}
	})
}
