package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
)

const maxFileListShow = 5

// CheckResult describes what the check hook detected.
type CheckResult struct {
	NeedsSync    bool
	NeedsRestart bool
	BenignSync   bool     // sync needed but no new unprotected files
	NewFiles     []string
	StateNewDeny []string // from previous sync
}

// Check runs the UserPromptSubmit hook logic.
// Returns nil result if everything is up to date (silent exit).
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

	if state.Hash == "" {
		result.NeedsSync = true
	} else {
		current := config.ComputeHash(root, mode)
		if current != state.Hash {
			result.NeedsSync = true
			result.NewFiles = findNewUnprotectedFiles(root, mode)
		}

		if state.Sync > 0 {
			parentStart := GetClaudeStartTime()
			if parentStart > 0 && state.Sync > parentStart {
				result.NeedsRestart = true
				result.StateNewDeny = state.NewDeny
			}
		}
	}

	// No new unprotected files and no restart = low-priority sync reminder
	if result.NeedsSync && len(result.NewFiles) == 0 && !result.NeedsRestart {
		result.BenignSync = true
	}

	if !result.NeedsSync && !result.NeedsRestart {
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

	if r.BenignSync {
		msg.WriteString("claudeignore: rules changed, run 'claudeignore sync' when convenient.")
		return msg.String()
	}

	msg.WriteString("\U0001F6A8 claudeignore: ")

	if r.NeedsSync && r.NeedsRestart {
		msg.WriteString("new files detected and restart pending.\n\n")
	} else if r.NeedsSync {
		msg.WriteString("ignore rules are out of sync.\n\n")
	} else {
		msg.WriteString("restart pending.\n\n")
	}

	if len(r.NewFiles) > 0 {
		msg.WriteString("New unprotected files:\n")
		writeFileList(&msg, r.NewFiles)
	} else if r.NeedsRestart && len(r.StateNewDeny) > 0 {
		msg.WriteString("New files pending restart:\n")
		writeFileList(&msg, r.StateNewDeny)
	}

	if r.NeedsSync {
		msg.WriteString("Run 'claudeignore sync' then restart Claude Code.")
	} else {
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

// GetClaudeStartTime is implemented in platform-specific files:
//   - check_unix.go   (linux, darwin, freebsd, etc.)
//   - check_windows.go (windows — graceful degradation, returns 0)
