package hooks

import (
	"os"
	"path/filepath"
	"time"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
)

// AutoSync performs a silent sync of the deny list, updating settings and state.
// It is called by the check hook when drift is detected.
// Returns the number of entries synced and any new files added.
func AutoSync(root, mode string) (synced int, newFiles []string, err error) {
	var deny []string

	if mode == "manual" {
		if git.HasGit(root) {
			allPaths, err := git.AllIgnoredPaths(root)
			if err != nil {
				return 0, nil, err
			}
			gitPaths, _ := git.GitIgnoredPaths(root)
			gitSet := config.NewPathSet(gitPaths)
			seen := config.NewPathSet(nil)

			for _, p := range allPaths {
				n := config.Normalize(p)
				if !config.PathSetContains(gitSet, p) && !config.PathSetContains(seen, n) {
					deny = append(deny, n)
					seen[n] = struct{}{}
				}
			}
		} else {
			deny, err = git.ManualDenyPaths(root)
			if err != nil {
				return 0, nil, err
			}
		}
	} else {
		paths, err := git.AllIgnoredPaths(root)
		if err != nil {
			return 0, nil, err
		}

		notignore := config.ReadLines(filepath.Join(root, ".claude.unignore"))
		notignoreSet := config.NewPathSet(notignore)

		for _, p := range paths {
			if !config.PathMatchesSet(notignoreSet, p) {
				deny = append(deny, config.Normalize(p))
			}
		}
	}

	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		return 0, nil, err
	}
	if err := config.EnsureClaudeGitignore(root); err != nil {
		return 0, nil, err
	}

	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Read previous deny list for diff
	var prevDenySet map[string]struct{}
	if settings, err := config.LoadSettings(settingsPath); err == nil {
		prevDenySet = config.NewPathSet(settings.GetDenyList())
	} else {
		prevDenySet = config.NewPathSet(nil)
	}

	if err := config.UpdateSettingsFile(settingsPath, deny); err != nil {
		return 0, nil, err
	}

	// Compute new files (in deny but not in prevDeny)
	var newDeny []string
	for _, d := range deny {
		if !config.PathSetContains(prevDenySet, d) {
			newDeny = append(newDeny, d)
		}
	}

	hash := config.ComputeHash(root, mode)
	if err := config.SaveState(root, config.StateData{
		Mode:    mode,
		Hash:    hash,
		Sync:    time.Now().Unix(),
		NewDeny: newDeny,
	}); err != nil {
		return 0, nil, err
	}

	return len(deny), newDeny, nil
}
