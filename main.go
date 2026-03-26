package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"crypto/sha256"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const version = "0.0.1-alpha"

// --- Git helpers ---

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func parseIgnoredOutput(out []byte) []string {
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "!! ") {
			continue
		}
		path := strings.TrimPrefix(line, "!! ")
		path = strings.TrimSuffix(path, "/")
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// gitIgnoredPaths returns paths ignored by .gitignore only.
func gitIgnoredPaths(root string) ([]string, error) {
	cmd := exec.Command("git", "status", "--ignored=traditional", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	return parseIgnoredOutput(out), nil
}

// allIgnoredPaths returns paths ignored by .gitignore + .claude.ignore combined,
// using git's own pattern engine via core.excludesFile.
func allIgnoredPaths(root string) ([]string, error) {
	claudeignorePath := filepath.Join(root, ".claude.ignore")
	if _, err := os.Stat(claudeignorePath); os.IsNotExist(err) {
		return gitIgnoredPaths(root)
	}

	absPath, _ := filepath.Abs(claudeignorePath)
	cmd := exec.Command("git", "-c", "core.excludesFile="+absPath, "status", "--ignored=traditional", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	return parseIgnoredOutput(out), nil
}

// --- .claude/.gitignore management ---

// Ensure .claude/.gitignore exists and contains local-only files
func ensureClaudeGitignore(root string) {
	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	gitignorePath := filepath.Join(claudeDir, ".gitignore")

	requiredEntries := []string{
		".claude.ignore.state.json",
		"settings.local.json",
	}

	// Read existing .gitignore
	var existing []string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				existing = append(existing, line)
			}
		}
	}

	// Add missing entries
	changed := false
	for _, entry := range requiredEntries {
		found := false
		for _, line := range existing {
			if line == entry {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, entry)
			changed = true
		}
	}

	if changed {
		content := strings.Join(existing, "\n") + "\n"
		os.WriteFile(gitignorePath, []byte(content), 0644)
	}
}

// --- File helpers ---

func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func writeLines(path string, header string, lines []string) error {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func normalize(s string) string {
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimPrefix(s, "/")
	return s
}

func contains(list []string, item string) bool {
	normalized := normalize(item)
	for _, v := range list {
		if normalize(v) == normalized {
			return true
		}
	}
	return false
}

// --- Settings JSON ---

func updateSettings(settingsPath string, denyOnly []string) error {
	// Read existing settings or start with empty object
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	}

	// Navigate/create sandbox.filesystem.denyRead
	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		sandbox = make(map[string]interface{})
	}
	filesystem, ok := sandbox["filesystem"].(map[string]interface{})
	if !ok {
		filesystem = make(map[string]interface{})
	}

	// Convert to []interface{} for JSON
	denyList := make([]interface{}, len(denyOnly))
	for i, v := range denyOnly {
		denyList[i] = v
	}
	filesystem["denyRead"] = denyList

	sandbox["filesystem"] = filesystem
	settings["sandbox"] = sandbox

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
}

// --- Hash ---

func computeHash(root string, mode string) string {
	h := sha256.New()
	if mode == "manual" {
		// Manual mode: hash only .claude.ignore
		data, err := os.ReadFile(filepath.Join(root, ".claude.ignore"))
		if err == nil {
			h.Write(data)
		}
	} else {
		// Gitignore mode: hash config files + git ignored file list
		for _, name := range []string{".gitignore", ".claude.unignore", ".claude.ignore"} {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err == nil {
				h.Write(data)
			}
		}
		paths, err := gitIgnoredPaths(root)
		if err == nil {
			for _, p := range paths {
				h.Write([]byte(p))
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

type stateData struct {
	Mode    string   `json:"mode"`              // "gitignore" or "manual"
	Hash    string   `json:"hash"`
	Sync    int64    `json:"sync"`
	NewDeny []string `json:"new_deny,omitempty"` // files added in last sync (diff with previous)
}

func stateFilePath(root string) string {
	return filepath.Join(root, ".claude", ".claude.ignore.state.json")
}

func loadState(root string) stateData {
	data, err := os.ReadFile(stateFilePath(root))
	if err != nil {
		return stateData{}
	}
	var s stateData
	json.Unmarshal(data, &s)
	return s
}

func saveState(root string, s stateData) error {
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(root), append(out, '\n'), 0644)
}

// Walk up the process tree to find the "claude" process and return its start time
func getClaudeStartTime() int64 {
	pid := os.Getppid()
	for i := 0; i < 10; i++ { // max 10 levels up
		if pid <= 1 {
			break
		}
		// Get comm and ppid for this process
		out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "ppid=,lstart=,comm=").Output()
		if err != nil {
			break
		}
		fields := strings.TrimSpace(string(out))
		// Format: "  26166 Thu Mar 26 21:43:36 2026 claude"
		// Split: ppid, then date (5 fields), then comm
		parts := strings.Fields(fields)
		if len(parts) < 7 {
			break
		}
		ppid, _ := strconv.Atoi(parts[0])
		comm := parts[len(parts)-1]
		// Extract lstart (between ppid and comm)
		dateStr := strings.Join(parts[1:len(parts)-1], " ")

		if strings.Contains(comm, "claude") {
			loc := time.Now().Location()
			t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", dateStr, loc)
			if err != nil {
				t, err = time.ParseInLocation("Mon Jan  2 15:04:05 2006", dateStr, loc)
			}
			if err == nil {
				return t.Unix()
			}
			return 0
		}
		pid = ppid
	}
	return 0
}


// --- TUI Model ---

type item struct {
	path    string
	checked bool // checked = blocked for Claude
}

type model struct {
	items      []item
	filtered   []int // indices into items
	cursor     int
	filter     textinput.Model
	quitting   bool
	confirmed  bool
	height     int
	scrollTop  int
}

var (
	checkedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	uncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	cursorStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
)

func newModel(paths []string, notignore []string) model {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 100

	items := make([]item, len(paths))
	for i, p := range paths {
		items[i] = item{
			path:    p,
			checked: !contains(notignore, p), // unchecked = in .claude.unignore
		}
	}

	m := model{
		items:  items,
		filter: ti,
		height: 20,
	}
	m.applyFilter()
	return m
}

func (m *model) applyFilter() {
	query := strings.ToLower(m.filter.Value())
	m.filtered = nil
	for i, it := range m.items {
		if query == "" || strings.Contains(strings.ToLower(it.path), query) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.scrollTop = 0
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 6 // reserve for header, filter, footer
		if m.height < 5 {
			m.height = 5
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "esc"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.confirmed = true
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
				m.items[idx].checked = !m.items[idx].checked
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

func (m model) View() string {
	if m.quitting {
		return "Cancelled.\n"
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("claudeignore — Select files to block from Claude Code"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[x] = blocked   [ ] = Claude can read   space = toggle   enter = confirm   esc = cancel"))
	b.WriteString("\n")
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matches"))
		b.WriteString("\n")
	} else {
		end := m.scrollTop + m.height
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for vi := m.scrollTop; vi < end; vi++ {
			idx := m.filtered[vi]
			it := m.items[idx]

			cursor := "  "
			if vi == m.cursor {
				cursor = "> "
			}

			var line string
			if it.checked {
				line = checkedStyle.Render("[x] " + it.path)
			} else {
				line = uncheckedStyle.Render("[ ] " + it.path)
			}

			if vi == m.cursor {
				line = cursorStyle.Render(cursor) + line
			} else {
				line = dimStyle.Render(cursor) + line
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Footer
	blocked := 0
	allowed := 0
	for _, it := range m.items {
		if it.checked {
			blocked++
		} else {
			allowed++
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d blocked, %d allowed, %d total", blocked, allowed, len(m.items))))
	b.WriteString("\n")

	return b.String()
}

// --- Init mode selector TUI ---

type modeItem struct {
	name string
	desc string
}

type modeModel struct {
	items    []modeItem
	cursor   int
	chosen   string
	quitting bool
}

var modeItems = []modeItem{
	{"gitignore", "From .gitignore (recommended)"},
	{"manual", "Manual (.claude.ignore only)"},
}

func (m modeModel) Init() tea.Cmd { return nil }

func (m modeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
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
			m.chosen = m.items[m.cursor].name
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m modeModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("claudeignore init — Choose mode"))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
			b.WriteString(cursorStyle.Render(cursor + it.desc))
		} else {
			b.WriteString(dimStyle.Render(cursor) + it.desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  enter = select   esc = cancel"))
	b.WriteString("\n")
	return b.String()
}

// --- Commands ---

func cmdInit() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	// Step 1: Choose mode
	mm := modeModel{items: modeItems}
	mp := tea.NewProgram(mm)
	mResult, err := mp.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	mFinal := mResult.(modeModel)
	if mFinal.quitting || mFinal.chosen == "" {
		return
	}
	mode := mFinal.chosen

	claudeignorePath := filepath.Join(root, ".claude.ignore")

	if mode == "manual" {
		// Create .claude.ignore if it doesn't exist
		if _, err := os.Stat(claudeignorePath); os.IsNotExist(err) {
			writeLines(claudeignorePath,
				"# .claude.ignore — Paths to block Claude from reading\n"+
					"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
				nil)
			fmt.Println("Created .claude.ignore")
		}

		// Save state with mode
		ensureClaudeGitignore(root)
		hash := computeHash(root, "manual")
		saveState(root, stateData{
			Mode: "manual",
			Hash: hash,
			Sync: time.Now().Unix(),
		})

		fmt.Println()
		fmt.Println("Manual mode enabled.")
		fmt.Println("Edit .claude.ignore then run 'claudeignore sync'.")
	} else {
		// Gitignore mode — existing flow
		paths, err := gitIgnoredPaths(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		if len(paths) == 0 {
			fmt.Println("No ignored files found by git.")
			return
		}

		// Load existing .claude.unignore for pre-selection
		notignorePath := filepath.Join(root, ".claude.unignore")
		notignore := readLines(notignorePath)

		// Run TUI
		m := newModel(paths, notignore)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		final := result.(model)
		if !final.confirmed {
			return
		}

		// Collect unchecked items → .claude.unignore
		var allowed []string
		for _, it := range final.items {
			if !it.checked {
				allowed = append(allowed, it.path)
			}
		}

		err = writeLines(notignorePath,
			"# .claude.unignore — Paths from .gitignore that Claude CAN read\n"+
				"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
			allowed)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error writing .claude.unignore:", err)
			os.Exit(1)
		}

		fmt.Printf("Saved %d allowed path(s) to .claude.unignore\n", len(allowed))

		// Create .claude.ignore if it doesn't exist
		if _, err := os.Stat(claudeignorePath); os.IsNotExist(err) {
			writeLines(claudeignorePath,
				"# .claude.ignore — Extra paths to block Claude from reading\n"+
					"# Same syntax as .gitignore — https://github.com/Claudeous/claudeignore",
				nil)
			fmt.Println("Created .claude.ignore")
		}

		// Auto-sync
		fmt.Println()
		cmdSyncWithMode("gitignore", false)
	}

	// Always install hooks at the end
	fmt.Println()
	cmdInstallHook()
}

func cmdSync() {
	// Detect --dry-run flag
	dryRun := false
	for _, arg := range os.Args[2:] {
		if arg == "--dry-run" {
			dryRun = true
		}
	}

	// Read mode from state
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	state := loadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore" // default for repos initialized before mode support
	}

	cmdSyncWithMode(mode, dryRun)
}

func cmdSyncWithMode(mode string, dryRun bool) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	var deny []string

	if mode == "manual" {
		// Manual mode: resolve .claude.ignore patterns via git, exclude .gitignore matches
		allPaths, err := allIgnoredPaths(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		gitPaths, _ := gitIgnoredPaths(root)

		for _, p := range allPaths {
			n := normalize(p)
			if !contains(gitPaths, p) && !contains(deny, n) {
				deny = append(deny, n)
			}
		}
	} else {
		// Gitignore mode: (git_ignored + claudeignore resolved) - claudenotignore
		paths, err := allIgnoredPaths(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		notignore := readLines(filepath.Join(root, ".claude.unignore"))

		for _, p := range paths {
			if !contains(notignore, p) {
				deny = append(deny, normalize(p))
			}
		}
	}

	if dryRun {
		fmt.Printf("[dry-run] Would sync %d entry(ies) to sandbox.filesystem.denyRead\n", len(deny))
		for _, d := range deny {
			fmt.Printf("  - %s\n", d)
		}
		return
	}

	// Ensure .claude directory exists with proper .gitignore
	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	ensureClaudeGitignore(root)

	// Read previous deny list from settings before overwriting
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	var prevDeny []string
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings map[string]interface{}
		if json.Unmarshal(data, &settings) == nil {
			if sandbox, ok := settings["sandbox"].(map[string]interface{}); ok {
				if fs, ok := sandbox["filesystem"].(map[string]interface{}); ok {
					if d, ok := fs["denyRead"].([]interface{}); ok {
						for _, v := range d {
							if s, ok := v.(string); ok {
								prevDeny = append(prevDeny, s)
							}
						}
					}
				}
			}
		}
	}

	// Update settings.local.json
	if err := updateSettings(settingsPath, deny); err != nil {
		fmt.Fprintln(os.Stderr, "Error updating settings:", err)
		os.Exit(1)
	}

	// Compute new files (in deny but not in prevDeny)
	var newDeny []string
	for _, d := range deny {
		if !contains(prevDeny, d) {
			newDeny = append(newDeny, d)
		}
	}

	// Save state
	hash := computeHash(root, mode)
	saveState(root, stateData{
		Mode:    mode,
		Hash:    hash,
		Sync:    time.Now().Unix(),
		NewDeny: newDeny,
	})

	fmt.Printf("Synced: %d entry(ies) to sandbox.filesystem.denyRead\n", len(deny))
	for _, d := range deny {
		fmt.Printf("  - %s\n", d)
	}
	fmt.Println()
	fmt.Println("Restart Claude Code to apply changes.")
}

func cmdCheck() {
	root, err := repoRoot()
	if err != nil {
		os.Exit(0)
	}

	state := loadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}

	// Detect two independent conditions
	needsSync := false
	needsRestart := false
	var newFiles []string

	// Never synced
	if state.Hash == "" {
		needsSync = true
	} else {
		// Rules changed on disk?
		current := computeHash(root, mode)
		if current != state.Hash {
			needsSync = true

			// Find new unprotected files
			var expected []string
			if mode == "manual" {
				allPaths, _ := allIgnoredPaths(root)
				gitPaths, _ := gitIgnoredPaths(root)
				for _, p := range allPaths {
					n := normalize(p)
					if !contains(gitPaths, p) && !contains(expected, n) {
						expected = append(expected, n)
					}
				}
			} else {
				paths, _ := allIgnoredPaths(root)
				notignore := readLines(filepath.Join(root, ".claude.unignore"))
				for _, p := range paths {
					if !contains(notignore, p) {
						expected = append(expected, normalize(p))
					}
				}
			}

			// Files expected but not yet in settings denyRead
			var currentDeny []string
			settingsPath := filepath.Join(root, ".claude", "settings.local.json")
			if data, err := os.ReadFile(settingsPath); err == nil {
				var settings map[string]interface{}
				if json.Unmarshal(data, &settings) == nil {
					if sandbox, ok := settings["sandbox"].(map[string]interface{}); ok {
						if fs, ok := sandbox["filesystem"].(map[string]interface{}); ok {
							if deny, ok := fs["denyRead"].([]interface{}); ok {
								for _, d := range deny {
									if s, ok := d.(string); ok {
										currentDeny = append(currentDeny, s)
									}
								}
							}
						}
					}
				}
			}
			for _, e := range expected {
				if !contains(currentDeny, e) {
					newFiles = append(newFiles, e)
				}
			}
		}

		// Sync happened after Claude Code started?
		if state.Sync > 0 {
			parentStart := getClaudeStartTime()
			if parentStart > 0 && state.Sync > parentStart {
				needsRestart = true
			}
		}
	}

	// No new unprotected files = benign change (file removed, etc.) — auto-sync silently
	if needsSync && len(newFiles) == 0 && !needsRestart {
		os.Exit(0)
	}

	if !needsSync && !needsRestart {
		os.Exit(0)
	}

	// Build message
	var msg strings.Builder
	msg.WriteString("\U0001F6A8 claudeignore: ")

	if needsSync && needsRestart {
		msg.WriteString("new files detected and restart pending.\n\n")
	} else if needsSync {
		msg.WriteString("ignore rules are out of sync.\n\n")
	} else {
		msg.WriteString("restart pending.\n\n")
	}

	// Show relevant files
	if len(newFiles) > 0 {
		msg.WriteString("New unprotected files:\n")
		writeFileList(&msg, newFiles)
	} else if needsRestart && len(state.NewDeny) > 0 {
		msg.WriteString("New files pending restart:\n")
		writeFileList(&msg, state.NewDeny)
	}

	if needsSync {
		msg.WriteString("Run 'claudeignore sync' then restart Claude Code.")
	} else {
		msg.WriteString("Restart Claude Code to apply Bash sandbox protection.\n(Read/Write/Edit are already protected)")
	}

	outputHookMessage(msg.String())
	os.Exit(0)
}

func writeFileList(b *strings.Builder, files []string) {
	maxShow := 5
	for i, f := range files {
		if i >= maxShow {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(files)-maxShow))
			break
		}
		b.WriteString(fmt.Sprintf("  - %s\n", f))
	}
	b.WriteString("\n")
}

func outputHookMessage(message string) {
	result := map[string]interface{}{
		"continue":       true,
		"suppressOutput": false,
		"systemMessage":  message,
	}
	out, _ := json.Marshal(result)
	fmt.Println(string(out))
}

// cmdGuard is the PreToolUse hook handler.
// It reads JSON from stdin, extracts the file path, and checks against denyRead.
// Exit 2 + JSON on stderr to block, exit 0 to allow.
func cmdGuard() {
	root, err := repoRoot()
	if err != nil {
		os.Exit(0) // can't determine root, allow
	}

	// Read deny list from settings.local.json
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		os.Exit(0)
	}

	var settings map[string]interface{}
	if json.Unmarshal(data, &settings) != nil {
		os.Exit(0)
	}

	// Extract denyRead list
	var denyList []string
	if sandbox, ok := settings["sandbox"].(map[string]interface{}); ok {
		if fs, ok := sandbox["filesystem"].(map[string]interface{}); ok {
			if deny, ok := fs["denyRead"].([]interface{}); ok {
				for _, d := range deny {
					if s, ok := d.(string); ok {
						denyList = append(denyList, s)
					}
				}
			}
		}
	}

	if len(denyList) == 0 {
		os.Exit(0)
	}

	// Read hook input from stdin
	input, err := os.ReadFile("/dev/stdin")
	if err != nil {
		os.Exit(0)
	}

	var hookInput map[string]interface{}
	if json.Unmarshal(input, &hookInput) != nil {
		os.Exit(0)
	}

	// Extract file path from tool_input (Read/Write/Edit use file_path, Grep/Glob use path)
	toolInput, ok := hookInput["tool_input"].(map[string]interface{})
	if !ok {
		os.Exit(0)
	}

	var targetPath string
	if fp, ok := toolInput["file_path"].(string); ok {
		targetPath = fp
	} else if p, ok := toolInput["path"].(string); ok {
		targetPath = p
	} else if p, ok := toolInput["pattern"].(string); ok {
		// Glob uses pattern, but that's a glob pattern not a path — skip
		_ = p
		os.Exit(0)
	}

	if targetPath == "" {
		os.Exit(0)
	}

	// Resolve to relative path from repo root
	absRoot, _ := filepath.Abs(root)
	absTarget, _ := filepath.Abs(targetPath)
	relPath, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		os.Exit(0)
	}

	// Check if path matches any deny entry (prefix matching like sandbox)
	for _, deny := range denyList {
		denyNorm := normalize(deny)
		if relPath == denyNorm || strings.HasPrefix(relPath, denyNorm+"/") {
			// Blocked
			result := map[string]string{
				"decision": "deny",
				"reason":   fmt.Sprintf("[claudeignore] Access denied: %s is in denyRead list", relPath),
			}
			out, _ := json.Marshal(result)
			fmt.Fprintln(os.Stderr, string(out))
			os.Exit(2)
		}
	}

	os.Exit(0)
}

func userHooksConfig() map[string]interface{} {
	return map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "Read|Write|Edit|Grep|Glob",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "claudeignore guard",
					},
				},
			},
		},
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "claudeignore check",
					},
				},
			},
		},
	}
}

func projectHooksConfig() map[string]interface{} {
	// Project scope: only check if claudeignore is installed, warn teammates if not
	checkScript := `which claudeignore >/dev/null 2>&1 && exit 0; echo '{"continue":true,"suppressOutput":false,"systemMessage":"` +
		`\u26a0\ufe0f This project uses claudeignore to protect sensitive files. ` +
		`Install it: brew tap claudeous/tools \u0026\u0026 brew install claudeignore ` +
		`— https://github.com/Claudeous/claudeignore"}'`

	return map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": checkScript,
					},
				},
			},
		},
	}
}

func installHooksToFile(path string, hooks map[string]interface{}) error {
	os.MkdirAll(filepath.Dir(path), 0755)

	var settings map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		json.Unmarshal(data, &settings)
		if settings == nil {
			settings = make(map[string]interface{})
		}
	}

	settings["hooks"] = hooks

	out, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}

func cmdInstallHook() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	ensureClaudeGitignore(root)

	// User scope: actual guard + check hooks
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: cannot find home directory:", err)
		os.Exit(1)
	}
	userSettingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := installHooksToFile(userSettingsPath, userHooksConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing user hooks:", err)
		os.Exit(1)
	}
	fmt.Println("User hooks installed in ~/.claude/settings.json")
	fmt.Println("  - PreToolUse: claudeignore guard")
	fmt.Println("  - UserPromptSubmit: claudeignore check")

	// Project scope: install check for teammates
	projectSettingsPath := filepath.Join(root, ".claude", "settings.json")
	if err := installHooksToFile(projectSettingsPath, projectHooksConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing project hooks:", err)
		os.Exit(1)
	}
	fmt.Println("Project hooks installed in .claude/settings.json")
	fmt.Println("  - UserPromptSubmit: install check (warns teammates)")

	fmt.Println()
	fmt.Println("Restart Claude Code to apply.")
}

func cmdStatus() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Printf("claudeignore v%s\n", version)
	fmt.Printf("Project: %s\n\n", root)

	// Mode
	state := loadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}
	fmt.Printf("Mode: %s\n", mode)

	// .claude.unignore (only relevant in gitignore mode)
	if mode != "manual" {
		notignore := readLines(filepath.Join(root, ".claude.unignore"))
		if notignore != nil {
			fmt.Printf(".claude.unignore: %d path(s) (allowed)\n", len(notignore))
		} else {
			fmt.Println(".claude.unignore: not found")
		}
	}

	// .claude.ignore
	extra := readLines(filepath.Join(root, ".claude.ignore"))
	if extra != nil {
		fmt.Printf(".claude.ignore: %d path(s) (extra deny)\n", len(extra))
	} else {
		fmt.Println(".claude.ignore: not found")
	}

	// Sync
	if state.Hash == "" {
		fmt.Println("Sync: never run")
	} else {
		current := computeHash(root, mode)
		if current == state.Hash {
			fmt.Println("Sync: up to date")
		} else {
			fmt.Println("Sync: out of date (run 'claudeignore sync')")
		}
	}

	// Sandbox
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings map[string]interface{}
		if json.Unmarshal(data, &settings) == nil {
			if sandbox, ok := settings["sandbox"].(map[string]interface{}); ok {
				if fs, ok := sandbox["filesystem"].(map[string]interface{}); ok {
					if deny, ok := fs["denyRead"].([]interface{}); ok {
						fmt.Printf("Sandbox denyRead: %d entry(ies)\n", len(deny))
					}
				}
			}
		}
	} else {
		fmt.Println("Sandbox: not configured")
	}

	// Hooks
	home, _ := os.UserHomeDir()
	userHook := false
	if home != "" {
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil {
			userHook = strings.Contains(string(data), "claudeignore check")
		}
	}
	projectHook := false
	if data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json")); err == nil {
		projectHook = strings.Contains(string(data), "claudeignore check")
	}

	if userHook && projectHook {
		fmt.Println("Hooks: user + project")
	} else if userHook {
		fmt.Println("Hooks: user only")
	} else if projectHook {
		fmt.Println("Hooks: project only")
	} else {
		fmt.Println("Hooks: not installed (run 'claudeignore install-hook')")
	}
}

func cmdHelp() {
	fmt.Println(`claudeignore — Sync gitignore rules to Claude Code sandbox

Modes:
  gitignore (default)   sandbox = git_ignored - .claude.unignore + .claude.ignore
  manual                sandbox = .claude.ignore only

Usage:
  claudeignore init              Interactive setup (choose mode, configure, install hooks)
  claudeignore sync              Apply current rules to sandbox
  claudeignore sync --dry-run    Preview deny list without writing
  claudeignore check             Check if rules changed (for hooks)
  claudeignore guard             Block tool access to denied files (for hooks)
  claudeignore install-hook      Install hooks (user + project scope)
  claudeignore status            Show current state
  claudeignore help              Show this help
  claudeignore version           Show version

Setup on a new project:
  1. claudeignore init     # Choose mode, configure, hooks auto-installed
  2. Restart Claude Code

Pattern syntax: same as .gitignore (see git-scm.com/docs/gitignore)

Requirements: git`)
}

// --- Menu TUI ---

type menuItem struct {
	name string
	desc string
}

type menuModel struct {
	items    []menuItem
	cursor   int
	chosen   string
	quitting bool
}

var menuItems = []menuItem{
	{"init", "Interactive TUI to select what Claude can read"},
	{"sync", "Apply current rules to sandbox"},
	{"check", "Check if rules changed (for hooks)"},
	{"guard", "Block tool access to denied files (for hooks)"},
	{"status", "Show current state"},
	{"install-hook", "Install all hooks in .claude/settings.json"},
	{"help", "Show help"},
	{"version", "Show version"},
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
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
			m.chosen = m.items[m.cursor].name
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m menuModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("claudeignore v"+version))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Sync gitignore rules to Claude Code sandbox"))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
			line := cursorStyle.Render(cursor + it.name)
			b.WriteString(line)
			b.WriteString("  ")
			b.WriteString(dimStyle.Render(it.desc))
		} else {
			b.WriteString(dimStyle.Render(cursor))
			b.WriteString(it.name)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  enter = select   esc = quit"))
	b.WriteString("\n")

	return b.String()
}

func runMenu() string {
	// Show status first
	cmdStatus()
	fmt.Println()

	m := menuModel{items: menuItems}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return ""
	}
	final := result.(menuModel)
	return final.chosen
}

func runCommand(cmd string) {
	switch cmd {
	case "init":
		cmdInit()
	case "sync":
		cmdSync()
	case "check":
		cmdCheck()
	case "guard":
		cmdGuard()
	case "install-hook":
		cmdInstallHook()
	case "status":
		cmdStatus()
	case "version":
		fmt.Printf("claudeignore v%s\n", version)
	case "help", "--help", "-h":
		cmdHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'claudeignore help' for available commands.\n", cmd)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) > 1 {
		runCommand(os.Args[1])
		return
	}

	// No argument → interactive menu
	chosen := runMenu()
	if chosen != "" {
		fmt.Println()
		runCommand(chosen)
	}
}
