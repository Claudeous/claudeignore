package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
	"github.com/claudeous/claudeignore/internal/support"
	"github.com/claudeous/claudeignore/internal/tui"
)

// Init runs the interactive setup wizard.
func Init(root string) error {
	// Step 1: Choose mode
	mm := tui.NewModeSelectorModel()
	mp := tea.NewProgram(mm)
	mResult, err := mp.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	mFinal := mResult.(tui.ModeSelectorModel)
	if mFinal.Quitting || mFinal.Chosen == "" {
		return nil
	}
	mode := mFinal.Chosen

	claudeignorePath := filepath.Join(root, ".claude.ignore")

	if mode == "manual" {
		if _, err := os.Stat(claudeignorePath); os.IsNotExist(err) {
			if err := config.WriteLines(claudeignorePath,
				"# .claude.ignore — Paths to block Claude from reading\n"+
					"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
				nil); err != nil {
				return fmt.Errorf("cannot create .claude.ignore: %w", err)
			}
			fmt.Println("Created .claude.ignore")
		}

		if err := config.EnsureClaudeGitignore(root); err != nil {
			return err
		}
		hash := config.ComputeHash(root, "manual")
		if err := config.SaveState(root, config.StateData{
			Mode: "manual",
			Hash: hash,
			Sync: time.Now().Unix(),
		}); err != nil {
			return fmt.Errorf("cannot save state: %w", err)
		}

		fmt.Println()
		fmt.Println("Manual mode enabled.")
		fmt.Println("Edit .claude.ignore then run 'claudeignore sync'.")
	} else {
		paths, err := git.GitIgnoredPaths(root)
		if err != nil {
			return err
		}

		if len(paths) == 0 {
			fmt.Println("No ignored files found by git.")
			return nil
		}

		notignorePath := filepath.Join(root, ".claude.unignore")
		notignore := config.ReadLines(notignorePath)

		m := tui.NewFilePickerModel(paths, notignore, root)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		final := result.(tui.FilePickerModel)
		if !final.Confirmed {
			return nil
		}

		allowed := final.AllowedPaths()

		if err := config.WriteLines(notignorePath,
			"# .claude.unignore — Paths from .gitignore that Claude CAN read\n"+
				"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
			allowed); err != nil {
			return fmt.Errorf("cannot write .claude.unignore: %w", err)
		}

		fmt.Printf("Saved %d allowed path(s) to .claude.unignore\n", len(allowed))

		if _, err := os.Stat(claudeignorePath); os.IsNotExist(err) {
			if err := config.WriteLines(claudeignorePath,
				"# .claude.ignore — Extra paths to block Claude from reading\n"+
					"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
				nil); err != nil {
				return fmt.Errorf("cannot create .claude.ignore: %w", err)
			}
			fmt.Println("Created .claude.ignore")
		}

		fmt.Println()
		if err := SyncWithMode(root, "gitignore", false); err != nil {
			return err
		}
	}

	fmt.Println()
	if err := InstallHook(root); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println(support.StyledMessage())
	return nil
}
