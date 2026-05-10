package hooks

import (
	"os"
	"path/filepath"
)

// NoGitWarnedSentinel returns the path to the sentinel file that tracks
// whether the "not inside a git repository" warning has already been shown
// to the user. Sitting under ~/.claude/claudeignore/, it is shared across
// all projects — the warning is shown once per user, not once per repo.
func NoGitWarnedSentinel() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "claudeignore", "no-git-warned"), nil
}

// WarnNotGitRepoOnce emits a hook message explaining that claudeignore is
// running outside a git repository, but only the first time it is called
// for this user. Subsequent invocations are silent so the warning does not
// spam every prompt when the user works outside a repo on purpose.
func WarnNotGitRepoOnce() {
	path, err := NoGitWarnedSentinel()
	if err != nil {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return
	}

	OutputHookMessage("claudeignore: running outside a git repository — hooks are idle here. (Shown once; delete ~/.claude/claudeignore/no-git-warned to see it again.)")

	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return
	}
	_ = os.WriteFile(path, nil, 0600)
}
