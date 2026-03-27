package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
)

// FileItem represents a file or collapsed directory entry in the picker.
type FileItem struct {
	Path     string
	Checked  bool     // checked = blocked for Claude
	Count    int      // >0 means collapsed directory
	Children []string // original paths when collapsed
}

// IsDir returns true if this item is a collapsed directory.
func (f FileItem) IsDir() bool {
	return f.Count > 0
}

// DisplayPath returns the path to show in the TUI.
func (f FileItem) DisplayPath() string {
	if f.IsDir() {
		return fmt.Sprintf("%s/ (%d files)", f.Path, f.Count)
	}
	return f.Path
}

// FilePickerModel is the TUI for selecting which files to block.
type FilePickerModel struct {
	Items     []FileItem
	filtered  []int // indices into Items
	cursor    int
	filter    textinput.Model
	Quitting  bool
	Confirmed bool
	height    int
	scrollTop int
}

// NewFilePickerModel creates a file picker with given paths and unignore list.
// Paths are collapsed by directory when a directory has many files.
func NewFilePickerModel(paths []string, notignore []string) FilePickerModel {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 100

	notignoreSet := config.NewPathSet(notignore)

	collapsed := git.CollapsePaths(paths)
	items := make([]FileItem, len(collapsed))
	for i, cp := range collapsed {
		checked := true
		if cp.IsDir() {
			// A collapsed dir is unchecked only if ALL children are in the unignore list
			for _, child := range cp.Children {
				if !config.PathSetContains(notignoreSet, child) {
					checked = true
					break
				}
				checked = false
			}
		} else {
			checked = !config.PathSetContains(notignoreSet, cp.Path)
		}

		items[i] = FileItem{
			Path:     cp.Path,
			Checked:  checked,
			Count:    cp.Count,
			Children: cp.Children,
		}
	}

	m := FilePickerModel{
		Items:  items,
		filter: ti,
		height: 20,
	}
	m.applyFilter()
	return m
}

// AllowedPaths returns the list of paths the user unchecked (Claude CAN read).
// For collapsed directories, returns the directory pattern (e.g. "pdf/")
// instead of expanding all children — .claude.unignore uses gitignore syntax.
func (m FilePickerModel) AllowedPaths() []string {
	var allowed []string
	for _, it := range m.Items {
		if !it.Checked {
			if it.IsDir() {
				allowed = append(allowed, it.Path+"/")
			} else {
				allowed = append(allowed, it.Path)
			}
		}
	}
	return allowed
}

func (m *FilePickerModel) applyFilter() {
	query := strings.ToLower(m.filter.Value())
	m.filtered = nil
	for i, it := range m.Items {
		if query == "" || strings.Contains(strings.ToLower(it.Path), query) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.scrollTop = 0
}

func (m FilePickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 6
		if m.height < 5 {
			m.height = 5
		}

	case tea.KeyMsg:
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
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			if m.cursor >= m.scrollTop+m.height {
				m.scrollTop = m.cursor - m.height + 1
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
			if len(m.filtered) > 0 {
				idx := m.filtered[m.cursor]
				m.Items[idx].Checked = !m.Items[idx].Checked
			}

		default:
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter()
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
	b.WriteString(DimStyle.Render("[x] = blocked   [ ] = Claude can read   space = toggle   enter = confirm   esc = cancel"))
	b.WriteString("\n")
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(DimStyle.Render("  No matches"))
		b.WriteString("\n")
	} else {
		end := m.scrollTop + m.height
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for vi := m.scrollTop; vi < end; vi++ {
			idx := m.filtered[vi]
			it := m.Items[idx]

			cursor := "  "
			if vi == m.cursor {
				cursor = "> "
			}

			display := it.DisplayPath()
			var line string
			if it.Checked {
				line = CheckedStyle.Render("[x] " + display)
			} else {
				line = UncheckedStyle.Render("[ ] " + display)
			}

			if vi == m.cursor {
				line = CursorStyle.Render(cursor) + line
			} else {
				line = DimStyle.Render(cursor) + line
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Footer: count actual files (expanding collapsed dirs)
	blocked := 0
	allowed := 0
	for _, it := range m.Items {
		n := 1
		if it.IsDir() {
			n = it.Count
		}
		if it.Checked {
			blocked += n
		} else {
			allowed += n
		}
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d blocked, %d allowed, %d total", blocked, allowed, blocked+allowed)))
	b.WriteString("\n")

	return b.String()
}
