package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
	"github.com/claudeous/claudeignore/internal/support"
)

// Sync reads the mode from state and syncs the deny list.
func Sync(root string, dryRun bool) error {
	state := config.LoadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}
	return SyncWithMode(root, mode, dryRun)
}

// SyncWithMode syncs the deny list for the given mode.
func SyncWithMode(root string, mode string, dryRun bool) error {
	var deny []string

	if mode == "manual" {
		allPaths, err := git.AllIgnoredPaths(root)
		if err != nil {
			return err
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
		paths, err := git.AllIgnoredPaths(root)
		if err != nil {
			return err
		}

		notignore := config.ReadLines(filepath.Join(root, ".claude.unignore"))
		notignoreSet := config.NewPathSet(notignore)

		for _, p := range paths {
			if !config.PathMatchesSet(notignoreSet, p) {
				deny = append(deny, config.Normalize(p))
			}
		}
	}

	if dryRun {
		fmt.Printf("[dry-run] Would sync %d entry(ies) to sandbox.filesystem.denyRead\n", len(deny))
		for _, d := range deny {
			fmt.Printf("  - %s\n", d)
		}
		return nil
	}

	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("cannot create .claude directory: %w", err)
	}
	if err := config.EnsureClaudeGitignore(root); err != nil {
		return err
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
		return fmt.Errorf("error updating settings: %w", err)
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
		return fmt.Errorf("cannot save state: %w", err)
	}

	fmt.Printf("Synced: %d entry(ies) to sandbox.filesystem.denyRead\n", len(deny))
	for _, d := range deny {
		fmt.Printf("  - %s\n", d)
	}
	fmt.Println()
	fmt.Println("Restart Claude Code to apply changes.")
	if support.ShouldShow() {
		fmt.Println()
		fmt.Println(support.StyledMessage())
	}
	return nil
}

