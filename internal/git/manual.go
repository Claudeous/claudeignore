package git

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ManualDenyPaths resolves .claude.ignore patterns without git.
// Used when operating in manual mode outside a git repository.
func ManualDenyPaths(root string) ([]string, error) {
	patterns := readIgnoreLines(filepath.Join(root, ".claude.ignore"))
	if len(patterns) == 0 {
		return nil, nil
	}

	var result []string
	seen := make(map[string]struct{})

	addPath := func(p string) {
		p = normalize(p)
		if p != "" {
			if _, ok := seen[p]; !ok {
				result = append(result, p)
				seen[p] = struct{}{}
			}
		}
	}

	// Separate simple paths from glob patterns
	var globs []string
	for _, pattern := range patterns {
		pattern = strings.TrimSuffix(pattern, "/")
		if !containsGlob(pattern) {
			addPath(pattern)
		} else {
			globs = append(globs, pattern)
		}
	}

	// Resolve glob patterns by walking the filesystem
	if len(globs) > 0 {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == "." {
				return nil
			}

			// Skip .git directory
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}

			for _, pattern := range globs {
				if matchIgnorePattern(pattern, rel) {
					addPath(rel)
					break
				}
			}
			return nil
		})
	}

	return result, nil
}

// readIgnoreLines reads non-empty, non-comment lines from a file.
func readIgnoreLines(path string) []string {
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

// normalize strips leading and trailing slashes from a path.
func normalize(s string) string {
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimPrefix(s, "/")
	return s
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// matchIgnorePattern matches a gitignore-style pattern against a relative path.
// Supports: *.ext (basename match), **/pattern (any depth), dir/file (exact).
func matchIgnorePattern(pattern, path string) bool {
	// Handle ** prefix (match at any depth)
	if strings.HasPrefix(pattern, "**/") {
		sub := strings.TrimPrefix(pattern, "**/")
		if matched, _ := filepath.Match(sub, filepath.Base(path)); matched {
			return true
		}
		// Try matching against each suffix of the path
		for i := 0; i < len(path); i++ {
			if path[i] == '/' {
				if matched, _ := filepath.Match(sub, path[i+1:]); matched {
					return true
				}
			}
		}
		return false
	}

	// Pattern without slash: match against basename at any level (gitignore behavior)
	if !strings.Contains(pattern, "/") {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		return matched
	}

	// Pattern with slash: match against relative path
	matched, _ := filepath.Match(pattern, path)
	return matched
}
