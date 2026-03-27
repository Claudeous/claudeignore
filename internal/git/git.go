package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const cmdTimeout = 15 * time.Second

// RepoRoot returns the absolute path to the git repository root.
func RepoRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// ParseIgnoredOutput extracts ignored paths from `git status --porcelain` output.
func ParseIgnoredOutput(out []byte) []string {
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "!! ") {
			continue
		}
		path := strings.TrimPrefix(line, "!! ")
		path = strings.TrimSuffix(path, "/")
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// GitIgnoredPaths returns paths ignored by .gitignore only.
func GitIgnoredPaths(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--ignored=traditional", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	return ParseIgnoredOutput(out), nil
}

// AllIgnoredPaths returns paths ignored by .gitignore + .claude.ignore combined,
// using git's own pattern engine via core.excludesFile.
func AllIgnoredPaths(root string) ([]string, error) {
	claudeignorePath := filepath.Join(root, ".claude.ignore")
	absPath, err := filepath.Abs(claudeignorePath)
	if err != nil {
		return GitIgnoredPaths(root)
	}

	// If .claude.ignore doesn't exist, fall back to git-only
	if !fileExists(absPath) {
		return GitIgnoredPaths(root)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-c", "core.excludesFile="+absPath, "status", "--ignored=traditional", "--porcelain") //nolint:gosec // absPath is a resolved local file path
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	return ParseIgnoredOutput(out), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
