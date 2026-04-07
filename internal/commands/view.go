package commands

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/tui"
)

// View launches a read-only TUI showing the current deny list.
func View(root string) error {
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	settings, err := config.LoadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("no settings found — run 'claudeignore sync' first")
	}

	denyList := settings.GetDenyList()
	if len(denyList) == 0 {
		fmt.Println("No files are currently blocked.")
		return nil
	}

	m := tui.NewDenyViewModel(denyList)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
