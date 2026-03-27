package git

import "testing"

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
