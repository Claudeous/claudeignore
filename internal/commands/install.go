package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/hooks"
)

// InstallHook installs hooks in both user and project scope.
func InstallHook(root string) error {
	if err := config.EnsureClaudeGitignore(root); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home directory: %w", err)
	}

	userSettingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := hooks.InstallHooksToFile(userSettingsPath, hooks.UserHooksConfig()); err != nil {
		return fmt.Errorf("error writing user hooks: %w", err)
	}
	fmt.Println("User hooks installed in ~/.claude/settings.json")
	fmt.Println("  - PreToolUse: claudeignore guard")
	fmt.Println("  - UserPromptSubmit: claudeignore check")

	projectSettingsPath := filepath.Join(root, ".claude", "settings.json")
	if err := hooks.InstallHooksToFile(projectSettingsPath, hooks.ProjectHooksConfig()); err != nil {
		return fmt.Errorf("error writing project hooks: %w", err)
	}
	fmt.Println("Project hooks installed in .claude/settings.json")
	fmt.Println("  - UserPromptSubmit: install check (warns teammates)")

	fmt.Println()
	fmt.Println("Restart Claude Code to apply.")
	return nil
}
