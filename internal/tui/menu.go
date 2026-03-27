package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MenuItem represents a menu option.
type MenuItem struct {
	Name string
	Desc string
}

// MenuModel is the main interactive menu TUI.
type MenuModel struct {
	Items    []MenuItem
	cursor   int
	Chosen   string
	Quitting bool
	Version  string
}

// NewMenuModel creates a menu with the given items and version string.
func NewMenuModel(items []MenuItem, version string) MenuModel {
	return MenuModel{Items: items, Version: version}
}

func (m MenuModel) Init() tea.Cmd { return nil }

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.Quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}
		case "enter":
			m.Chosen = m.Items[m.cursor].Name
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m MenuModel) View() string {
	if m.Quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(HeaderStyle.Render("claudeignore v" + m.Version))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("Sync gitignore rules to Claude Code sandbox"))
	b.WriteString("\n\n")

	for i, it := range m.Items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
			line := CursorStyle.Render(cursor + it.Name)
			b.WriteString(line)
			b.WriteString("  ")
			b.WriteString(DimStyle.Render(it.Desc))
		} else {
			b.WriteString(DimStyle.Render(cursor))
			b.WriteString(it.Name)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  enter = select   esc = quit"))
	b.WriteString("\n")

	return b.String()
}
