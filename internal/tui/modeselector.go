package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ModeItem represents a mode option.
type ModeItem struct {
	Name string
	Desc string
}

// ModeItems are the available modes.
var ModeItems = []ModeItem{
	{"gitignore", "From .gitignore (recommended)"},
	{"manual", "Manual (.claude.ignore only)"},
}

// ModeSelectorModel is the TUI for choosing a mode.
type ModeSelectorModel struct {
	items    []ModeItem
	cursor   int
	Chosen   string
	Quitting bool
}

// NewModeSelectorModel creates a mode selector.
func NewModeSelectorModel() ModeSelectorModel {
	return ModeSelectorModel{items: ModeItems}
}

func (m ModeSelectorModel) Init() tea.Cmd { return nil }

func (m ModeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.Chosen = m.items[m.cursor].Name
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ModeSelectorModel) View() string {
	if m.Quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(HeaderStyle.Render("claudeignore init — Choose mode"))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
			b.WriteString(CursorStyle.Render(cursor + it.Desc))
		} else {
			b.WriteString(DimStyle.Render(cursor) + it.Desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  enter = select   esc = cancel"))
	b.WriteString("\n")
	return b.String()
}
