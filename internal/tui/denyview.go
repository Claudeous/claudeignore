package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// DenyGroup represents a directory group in the deny list viewer.
type DenyGroup struct {
	Name     string
	Children []string
	Expanded bool
}

// DenyViewModel is a read-only TUI for viewing the current deny list.
type DenyViewModel struct {
	Groups   []DenyGroup
	cursor   int
	filter   textinput.Model
	Quitting bool
	height   int
	scroll   int
}

type denyRow struct {
	groupIdx int
	childIdx int // -1 = group header
}

// NewDenyViewModel creates a read-only tree view from a deny list.
func NewDenyViewModel(denyList []string) DenyViewModel {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 100

	groupMap := make(map[string][]string)
	var groupOrder []string
	var rootFiles []string

	for _, p := range denyList {
		idx := strings.IndexByte(p, '/')
		if idx == -1 {
			rootFiles = append(rootFiles, p)
			continue
		}
		dir := p[:idx]
		if _, seen := groupMap[dir]; !seen {
			groupOrder = append(groupOrder, dir)
		}
		groupMap[dir] = append(groupMap[dir], p)
	}

	var groups []DenyGroup

	if len(rootFiles) > 0 {
		groups = append(groups, DenyGroup{
			Name:     "",
			Children: rootFiles,
			Expanded: true,
		})
	}

	for _, dir := range groupOrder {
		files := groupMap[dir]
		groups = append(groups, DenyGroup{
			Name:     dir,
			Children: files,
			Expanded: len(files) < 20,
		})
	}

	return DenyViewModel{
		Groups: groups,
		filter: ti,
		height: 20,
	}
}

func (m DenyViewModel) visibleRows() []denyRow {
	query := strings.ToLower(m.filter.Value())
	var rows []denyRow

	for gi, g := range m.Groups {
		hasMatch := false
		if query == "" {
			hasMatch = true
		} else {
			for _, c := range g.Children {
				if strings.Contains(strings.ToLower(c), query) {
					hasMatch = true
					break
				}
			}
			if g.Name != "" && strings.Contains(strings.ToLower(g.Name), query) {
				hasMatch = true
			}
		}
		if !hasMatch {
			continue
		}

		if g.Name != "" {
			rows = append(rows, denyRow{groupIdx: gi, childIdx: -1})
		}

		if g.Expanded || g.Name == "" {
			for ci, c := range g.Children {
				if query != "" && !strings.Contains(strings.ToLower(c), query) {
					continue
				}
				rows = append(rows, denyRow{groupIdx: gi, childIdx: ci})
			}
		}
	}

	return rows
}

func (m DenyViewModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m DenyViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 6
		if m.height < 5 {
			m.height = 5
		}

	case tea.KeyMsg:
		rows := m.visibleRows()

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "esc", "q"))):
			m.Quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.cursor > 0 {
				m.cursor--
			}
			if m.cursor < m.scroll {
				m.scroll = m.cursor
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.cursor < len(rows)-1 {
				m.cursor++
			}
			if m.cursor >= m.scroll+m.height {
				m.scroll = m.cursor - m.height + 1
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("right"))):
			if m.cursor < len(rows) {
				r := rows[m.cursor]
				if r.childIdx == -1 {
					m.Groups[r.groupIdx].Expanded = true
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("left"))):
			if m.cursor < len(rows) {
				r := rows[m.cursor]
				if r.childIdx == -1 {
					m.Groups[r.groupIdx].Expanded = false
				}
			}

		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m DenyViewModel) View() string {
	var b strings.Builder

	b.WriteString(HeaderStyle.Render("claudeignore — Blocked files (deny list)"))
	b.WriteString("\n")
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	rows := m.visibleRows()

	if len(rows) == 0 {
		b.WriteString(DimStyle.Render("  No entries"))
		b.WriteString("\n")
	} else {
		if m.cursor >= len(rows) {
			m.cursor = len(rows) - 1
		}

		end := m.scroll + m.height
		if end > len(rows) {
			end = len(rows)
		}
		for vi := m.scroll; vi < end; vi++ {
			r := rows[vi]

			cursor := "  "
			if vi == m.cursor {
				cursor = "> "
			}

			if r.childIdx == -1 {
				g := m.Groups[r.groupIdx]
				arrow := "▸"
				if g.Expanded {
					arrow = "▾"
				}
				label := CheckedStyle.Render(fmt.Sprintf(" %s %s/ (%d files)", arrow, g.Name, len(g.Children)))
				if vi == m.cursor {
					b.WriteString(CursorStyle.Render(cursor) + label)
				} else {
					b.WriteString(DimStyle.Render(cursor) + label)
				}
			} else {
				c := m.Groups[r.groupIdx].Children[r.childIdx]
				indent := "  "
				if m.Groups[r.groupIdx].Name != "" {
					indent = "    "
				}

				line := CheckedStyle.Render(c)
				if vi == m.cursor {
					b.WriteString(CursorStyle.Render(cursor) + indent + line)
				} else {
					b.WriteString(DimStyle.Render(cursor) + indent + line)
				}
			}

			b.WriteString("\n")
		}
	}

	total := 0
	for _, g := range m.Groups {
		total += len(g.Children)
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  ←/→ = expand/collapse   q/esc = quit"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d blocked entries", total)))
	b.WriteString("\n")

	return b.String()
}
