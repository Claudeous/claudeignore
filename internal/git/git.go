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

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("not inside a git repository: %s", msg)
		}
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// HasGit returns true if the given directory is inside a git repository.
func HasGit(root string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	return cmd.Run() == nil
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

// ListSubmodules returns relative paths of all initialized git submodules.
// Returns nil (not an error) when there are no submodules or git fails.
func ListSubmodules(root string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "submodule", "--quiet", "foreach", "echo $displaypath")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// gitIgnoredPathsInDir returns ignored paths for a single git directory,
// prefixing each result with the given prefix (empty for root repo).
func gitIgnoredPathsInDir(dir, prefix string, extraArgs ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	args := append(extraArgs, "status", "--ignored=traditional", "--porcelain")
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // extraArgs are controlled internally (e.g. -c core.excludesFile=path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed in %s: %w", dir, err)
	}
	raw := ParseIgnoredOutput(out)
	if prefix == "" {
		return raw, nil
	}
	prefixed := make([]string, len(raw))
	for i, p := range raw {
		prefixed[i] = prefix + "/" + p
	}
	return prefixed, nil
}

// GitIgnoredPaths returns paths ignored by .gitignore, including inside submodules.
func GitIgnoredPaths(root string) ([]string, error) {
	paths, err := gitIgnoredPathsInDir(root, "")
	if err != nil {
		return nil, err
	}
	for _, sub := range ListSubmodules(root) {
		subPaths, err := gitIgnoredPathsInDir(filepath.Join(root, sub), sub)
		if err != nil {
			continue // skip broken submodules
		}
		paths = append(paths, subPaths...)
	}
	return paths, nil
}

// AllIgnoredPaths returns paths ignored by .gitignore + .claude.ignore combined,
// using git's own pattern engine via core.excludesFile. Includes submodule paths.
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

	extraArgs := []string{"-c", "core.excludesFile=" + absPath} //nolint:gosec // absPath is a resolved local file path
	paths, err := gitIgnoredPathsInDir(root, "", extraArgs...)
	if err != nil {
		return nil, err
	}
	for _, sub := range ListSubmodules(root) {
		subPaths, err := gitIgnoredPathsInDir(filepath.Join(root, sub), sub, extraArgs...)
		if err != nil {
			continue // skip broken submodules
		}
		paths = append(paths, subPaths...)
	}
	return paths, nil
}

// CollapsedPath represents either a single file or a collapsed directory.
type CollapsedPath struct {
	Path     string   // the display path (e.g. "pdf" for a collapsed dir, ".env" for a file)
	Count    int      // number of files collapsed (0 = single file)
	Children []string // original child paths when collapsed
}

// IsDir returns true if this entry is a collapsed directory.
func (c CollapsedPath) IsDir() bool {
	return c.Count > 0
}

const collapseThreshold = 5

// CollapsePaths groups paths by their first directory component.
// Directories with more than collapseThreshold files are collapsed into a single entry.
func CollapsePaths(paths []string) []CollapsedPath {
	groups := make(map[string][]string)
	var order []string // preserve first-seen order of directories
	var rootFiles []string

	for _, p := range paths {
		idx := strings.IndexByte(p, '/')
		if idx == -1 {
			rootFiles = append(rootFiles, p)
			continue
		}
		dir := p[:idx]
		if _, seen := groups[dir]; !seen {
			order = append(order, dir)
		}
		groups[dir] = append(groups[dir], p)
	}

	var result []CollapsedPath

	for _, f := range rootFiles {
		result = append(result, CollapsedPath{Path: f})
	}

	for _, dir := range order {
		files := groups[dir]
		if len(files) > collapseThreshold {
			result = append(result, CollapsedPath{
				Path:     dir,
				Count:    len(files),
				Children: files,
			})
		} else {
			for _, f := range files {
				result = append(result, CollapsedPath{Path: f})
			}
		}
	}

	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
