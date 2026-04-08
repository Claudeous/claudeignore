package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
)

const maxFileListShow = 5

// CheckResult describes what the check hook detected.
type CheckResult struct {
	NeedsRestart bool
	AutoSynced   bool     // true if auto-sync was performed
	SyncedCount  int      // number of entries synced
	NewFiles     []string // new files added by auto-sync
	StateNewDeny []string // from previous sync (for restart message)
}

// Check runs the UserPromptSubmit hook logic.
// When drift is detected, it auto-syncs the deny list so the guard hook
// picks up changes immediately. Returns nil if everything is up to date.
func Check(root string) (*CheckResult, error) {
	// Skip projects that were never initialized with claudeignore
	if _, err := os.Stat(config.StateFilePath(root)); os.IsNotExist(err) {
		return nil, nil
	}

	state := config.LoadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}

	result := &CheckResult{}

	// Detect drift
	needsSync := false
	if state.Hash == "" {
		needsSync = true
	} else {
		current := config.ComputeHash(root, mode)
		if current != state.Hash {
			needsSync = true
		}
	}

	// Auto-sync when drift detected
	if needsSync {
		synced, newFiles, err := AutoSync(root, mode)
		if err == nil {
			result.AutoSynced = true
			result.SyncedCount = synced
			result.NewFiles = newFiles
			// After sync, we always need a restart for Bash sandbox protection
			result.NeedsRestart = true
		}
		// If auto-sync fails, fall through silently (fail-open)
	}

	// Check if a previous sync still needs a restart
	if !needsSync && state.Sync > 0 {
		parentStart := GetClaudeStartTime()
		if parentStart > 0 && state.Sync > parentStart {
			result.NeedsRestart = true
			result.StateNewDeny = state.NewDeny
		}
	}

	if !result.AutoSynced && !result.NeedsRestart {
		return nil, nil
	}

	return result, nil
}

func findNewUnprotectedFiles(root, mode string) []string {
	var expected []string
	if mode == "manual" {
		if git.HasGit(root) {
			allPaths, _ := git.AllIgnoredPaths(root)
			gitPaths, _ := git.GitIgnoredPaths(root)
			gitSet := config.NewPathSet(gitPaths)
			seen := config.NewPathSet(nil)
			for _, p := range allPaths {
				n := config.Normalize(p)
				if !config.PathSetContains(gitSet, p) && !config.PathSetContains(seen, n) {
					expected = append(expected, n)
					seen[n] = struct{}{}
				}
			}
		} else {
			expected, _ = git.ManualDenyPaths(root)
		}
	} else {
		paths, _ := git.AllIgnoredPaths(root)
		notignore := config.ReadLines(filepath.Join(root, ".claude.unignore"))
		notignoreSet := config.NewPathSet(notignore)
		for _, p := range paths {
			if !config.PathMatchesSet(notignoreSet, p) {
				expected = append(expected, config.Normalize(p))
			}
		}
	}

	// Files expected but not yet in settings denyRead
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	settings, err := config.LoadSettings(settingsPath)
	var currentDenySet map[string]struct{}
	if err == nil {
		currentDenySet = config.NewPathSet(settings.GetDenyList())
	} else {
		currentDenySet = config.NewPathSet(nil)
	}

	var newFiles []string
	for _, e := range expected {
		if !config.PathSetContains(currentDenySet, e) {
			newFiles = append(newFiles, e)
		}
	}
	return newFiles
}

// FormatCheckMessage builds the user-facing alert message.
func FormatCheckMessage(r *CheckResult) string {
	var msg strings.Builder

	if r.AutoSynced {
		msg.WriteString("\u2705 claudeignore: auto-synced ")
		fmt.Fprintf(&msg, "%d entries.", r.SyncedCount)

		if len(r.NewFiles) > 0 {
			msg.WriteString("\n\nNew files now protected:\n")
			writeFileList(&msg, r.NewFiles)
		}

		msg.WriteString("\nRead/Write/Edit/Grep/Glob are protected immediately.")
		msg.WriteString("\nRestart Claude Code to also protect Bash commands.")
		return msg.String()
	}

	// Restart pending from a previous sync
	if r.NeedsRestart {
		msg.WriteString("\U0001F6A8 claudeignore: restart pending.\n\n")
		if len(r.StateNewDeny) > 0 {
			msg.WriteString("New files pending restart:\n")
			writeFileList(&msg, r.StateNewDeny)
		}
		msg.WriteString("Restart Claude Code to update Bash sandbox protection.\n(Read/Write/Edit are already protected)")
	}

	return msg.String()
}

func writeFileList(b *strings.Builder, files []string) {
	for i, f := range files {
		if i >= maxFileListShow {
			fmt.Fprintf(b, "  ... and %d more\n", len(files)-maxFileListShow)
			break
		}
		fmt.Fprintf(b, "  - %s\n", f)
	}
	b.WriteString("\n")
}

// GetClaudeStartTime walks up the process tree to find the "claude" process start time.
func GetClaudeStartTime() int64 {
	pid := os.Getppid()
	for i := 0; i < 10; i++ {
		if pid <= 1 {
			break
		}
		out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "ppid=,lstart=,comm=").Output() //nolint:gosec // pid is from os.Getppid
		if err != nil {
			break
		}
		fields := strings.TrimSpace(string(out))
		parts := strings.Fields(fields)
		if len(parts) < 7 {
			break
		}
		ppid, _ := strconv.Atoi(parts[0])
		comm := parts[len(parts)-1]
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
