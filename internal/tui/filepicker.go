package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/config"
)

const expandThreshold = 20

// FileChild represents an individual file inside a group.
type FileChild struct {
	Path    string
	Checked bool
}

// FileGroup represents a directory group or the root files group.
type FileGroup struct {
	Name     string // directory name or "" for root files
	Children []FileChild
	Expanded bool
}

// FilePickerModel is the TUI for selecting which files to block.
type FilePickerModel struct {
	Groups       []FileGroup
	initialState [][]bool // [group][child] checked state for restore
	cursor       int      // position in the flat visible list
	filter       textinput.Model
	Quitting     bool
	Confirmed    bool
	height       int
	scrollTop    int
}

// visibleRow represents one row in the flat visible list.
type visibleRow struct {
	groupIdx int
	childIdx int // -1 means this row is a group header
}

// NewFilePickerModel creates a file picker with given paths and unignore list.
func NewFilePickerModel(paths []string, notignore []string) FilePickerModel {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 100

	notignoreSet := config.NewPathSet(notignore)

	// Group files by first directory component
	groupMap := make(map[string][]string)
	var groupOrder []string
	var rootFiles []string

	for _, p := range paths {
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

	var groups []FileGroup

	// Root files group
	if len(rootFiles) > 0 {
		children := make([]FileChild, len(rootFiles))
		for i, f := range rootFiles {
			children[i] = FileChild{
				Path:    f,
				Checked: !config.PathSetContains(notignoreSet, f),
			}
		}
		groups = append(groups, FileGroup{
			Name:     "",
			Children: children,
			Expanded: true,
		})
	}

	// Directory groups
	for _, dir := range groupOrder {
		files := groupMap[dir]
		children := make([]FileChild, len(files))
		for i, f := range files {
			children[i] = FileChild{
				Path:    f,
				Checked: !config.PathMatchesSet(notignoreSet, f),
			}
		}
		groups = append(groups, FileGroup{
			Name:     dir,
			Children: children,
			Expanded: len(files) < expandThreshold,
		})
	}

	// Save initial state for restore
	initial := make([][]bool, len(groups))
	for i, g := range groups {
		initial[i] = make([]bool, len(g.Children))
		for j, c := range g.Children {
			initial[i][j] = c.Checked
		}
	}

	m := FilePickerModel{
		Groups:       groups,
		initialState: initial,
		filter:       ti,
		height:       20,
	}
	return m
}

// AllowedPaths returns unchecked paths (Claude CAN read).
// For fully unchecked directory groups, returns "dir/" pattern.
// For individually unchecked files, returns the file path.
func (m FilePickerModel) AllowedPaths() []string {
	var allowed []string
	for _, g := range m.Groups {
		if g.Name != "" {
			allUnchecked := true
			for _, c := range g.Children {
				if c.Checked {
					allUnchecked = false
					break
				}
			}
			if allUnchecked {
				allowed = append(allowed, g.Name+"/")
				continue
			}
		}
		for _, c := range g.Children {
			if !c.Checked {
				allowed = append(allowed, c.Path)
			}
		}
	}
	return allowed
}

// visibleRows returns the flat list of visible rows based on expand state and filter.
func (m FilePickerModel) visibleRows() []visibleRow {
	query := strings.ToLower(m.filter.Value())
	var rows []visibleRow

	for gi, g := range m.Groups {
		hasMatch := false
		if query == "" {
			hasMatch = true
		} else {
			for _, c := range g.Children {
				if strings.Contains(strings.ToLower(c.Path), query) {
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
			rows = append(rows, visibleRow{groupIdx: gi, childIdx: -1})
		}

		if g.Expanded || g.Name == "" {
			for ci, c := range g.Children {
				if query != "" && !strings.Contains(strings.ToLower(c.Path), query) {
					continue
				}
				rows = append(rows, visibleRow{groupIdx: gi, childIdx: ci})
			}
		}
	}

	return rows
}

func (m *FilePickerModel) selectAll() {
	for gi := range m.Groups {
		for ci := range m.Groups[gi].Children {
			m.Groups[gi].Children[ci].Checked = true
		}
	}
}

func (m *FilePickerModel) unselectAll() {
	for gi := range m.Groups {
		for ci := range m.Groups[gi].Children {
			m.Groups[gi].Children[ci].Checked = false
		}
	}
}

func (m *FilePickerModel) restore() {
	for gi := range m.Groups {
		for ci := range m.Groups[gi].Children {
			m.Groups[gi].Children[ci].Checked = m.initialState[gi][ci]
		}
	}
}

func (m *FilePickerModel) toggleGroup(gi int) {
	g := &m.Groups[gi]
	anyChecked := false
	for _, c := range g.Children {
		if c.Checked {
			anyChecked = true
			break
		}
	}
	for ci := range g.Children {
		g.Children[ci].Checked = !anyChecked
	}
}

func (m FilePickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 8
		if m.height < 5 {
			m.height = 5
		}

	case tea.KeyMsg:
		rows := m.visibleRows()

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "esc"))):
			m.Quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.Confirmed = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("up"))):
			if m.cursor > 0 {
				m.cursor--
			}
			if m.cursor < m.scrollTop {
				m.scrollTop = m.cursor
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down"))):
			if m.cursor < len(rows)-1 {
				m.cursor++
			}
			if m.cursor >= m.scrollTop+m.height {
				m.scrollTop = m.cursor - m.height + 1
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

		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
			if m.cursor < len(rows) {
				r := rows[m.cursor]
				if r.childIdx == -1 {
					m.toggleGroup(r.groupIdx)
				} else {
					m.Groups[r.groupIdx].Children[r.childIdx].Checked =
						!m.Groups[r.groupIdx].Children[r.childIdx].Checked
				}
			}

		case msg.String() == "a":
			m.selectAll()

		case msg.String() == "n":
			m.unselectAll()

		case msg.String() == "r":
			m.restore()

		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m FilePickerModel) View() string {
	if m.Quitting {
		return "Cancelled.\n"
	}

	var b strings.Builder

	b.WriteString(HeaderStyle.Render("claudeignore — Select files to block from Claude Code"))
	b.WriteString("\n")
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	rows := m.visibleRows()

	if len(rows) == 0 {
		b.WriteString(DimStyle.Render("  No matches"))
		b.WriteString("\n")
	} else {
		if m.cursor >= len(rows) {
			m.cursor = len(rows) - 1
		}

		end := m.scrollTop + m.height
		if end > len(rows) {
			end = len(rows)
		}
		for vi := m.scrollTop; vi < end; vi++ {
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

				checked := 0
				for _, c := range g.Children {
					if c.Checked {
						checked++
					}
				}

				var header string
				if checked == len(g.Children) {
					header = CheckedStyle.Render(fmt.Sprintf("%s %s/ (%d files)", arrow, g.Name, len(g.Children)))
				} else if checked == 0 {
					header = UncheckedStyle.Render(fmt.Sprintf("%s %s/ (%d files)", arrow, g.Name, len(g.Children)))
				} else {
					header = DimStyle.Render(fmt.Sprintf("%s %s/ (%d/%d blocked)", arrow, g.Name, checked, len(g.Children)))
				}

				if vi == m.cursor {
					b.WriteString(CursorStyle.Render(cursor) + header)
				} else {
					b.WriteString(DimStyle.Render(cursor) + header)
				}
			} else {
				c := m.Groups[r.groupIdx].Children[r.childIdx]
				indent := "  "
				if m.Groups[r.groupIdx].Name != "" {
					indent = "    "
				}

				var line string
				if c.Checked {
					line = CheckedStyle.Render("[x] " + c.Path)
				} else {
					line = UncheckedStyle.Render("[ ] " + c.Path)
				}

				if vi == m.cursor {
					b.WriteString(CursorStyle.Render(cursor) + indent + line)
				} else {
					b.WriteString(DimStyle.Render(cursor) + indent + line)
				}
			}

			b.WriteString("\n")
		}
	}

	blocked := 0
	allowed := 0
	for _, g := range m.Groups {
		for _, c := range g.Children {
			if c.Checked {
				blocked++
			} else {
				allowed++
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  space = toggle   \u2192/\u2190 = expand/collapse   a/n/r = all/none/restore   enter = confirm   esc = cancel"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d blocked, %d allowed, %d total", blocked, allowed, blocked+allowed)))
	b.WriteString("\n")

	return b.String()
}
