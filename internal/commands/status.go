package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/support"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(20)
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

const (
	iconOK   = "\u2714" // checkmark
	iconWarn = "\u25CB" // circle
	iconFail = "\u2718" // cross
)

func statusLine(label, icon, value string, style lipgloss.Style) string {
	return labelStyle.Render(label) + style.Render(icon+" "+value)
}

// Status prints the current state of claudeignore.
func Status(root string, version string, showSupport bool) error {
	fmt.Println(titleStyle.Render(fmt.Sprintf("claudeignore %s", version)))
	fmt.Println(dimStyle.Render(root))
	fmt.Println()

	state := config.LoadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}

	// Mode
	fmt.Println(statusLine("Mode", iconOK, mode, okStyle))

	// Config files
	if mode != "manual" {
		unignorePath := filepath.Join(root, ".claude.unignore")
		if _, err := os.Stat(unignorePath); err == nil {
			notignore := config.ReadLines(unignorePath)
			fmt.Println(statusLine(".claude.unignore", iconOK, fmt.Sprintf("%d path(s) allowed", len(notignore)), okStyle))
		} else {
			fmt.Println(statusLine(".claude.unignore", iconWarn, "not found", warnStyle))
		}
	}

	ignorePath := filepath.Join(root, ".claude.ignore")
	if _, err := os.Stat(ignorePath); err == nil {
		extra := config.ReadLines(ignorePath)
		fmt.Println(statusLine(".claude.ignore", iconOK, fmt.Sprintf("%d path(s) extra deny", len(extra)), okStyle))
	} else {
		fmt.Println(statusLine(".claude.ignore", iconWarn, "not found", warnStyle))
	}

	// Sync
	if state.Hash == "" {
		fmt.Println(statusLine("Sync", iconFail, "never run", errStyle))
	} else {
		current := config.ComputeHash(root, mode)
		if current == state.Hash {
			fmt.Println(statusLine("Sync", iconOK, "up to date", okStyle))
		} else {
			fmt.Println(statusLine("Sync", iconFail, "out of date — run 'claudeignore sync'", errStyle))
		}
	}

	// Sandbox
	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	if settings, err := config.LoadSettings(settingsPath); err == nil {
		denyList := settings.GetDenyList()
		fmt.Println(statusLine("Sandbox denyRead", iconOK, fmt.Sprintf("%d entry(ies)", len(denyList)), okStyle))
	} else {
		fmt.Println(statusLine("Sandbox denyRead", iconFail, "not configured", errStyle))
	}

	// Hooks
	home, _ := os.UserHomeDir()
	userHook := false
	if home != "" {
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil {
			userHook = strings.Contains(string(data), "claudeignore")
		}
	}
	projectHook := false
	if data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json")); err == nil {
		projectHook = strings.Contains(string(data), "claudeignore")
	}

	if userHook && projectHook {
		fmt.Println(statusLine("Hooks", iconOK, "user + project", okStyle))
	} else if userHook {
		fmt.Println(statusLine("Hooks", iconWarn, "user only (run 'claudeignore install-hook')", warnStyle))
	} else if projectHook {
		fmt.Println(statusLine("Hooks", iconWarn, "project only (run 'claudeignore install-hook')", warnStyle))
	} else {
		fmt.Println(statusLine("Hooks", iconFail, "not installed — run 'claudeignore install-hook'", errStyle))
	}

	// Binary & hook health checks
	fmt.Println()
	fmt.Println(titleStyle.Render("Hook health"))

	binaryPath, err := exec.LookPath("claudeignore")
	if err != nil {
		fmt.Println(statusLine("Binary", iconFail, "not found in PATH", errStyle))
	} else {
		fmt.Println(statusLine("Binary", iconOK, binaryPath, okStyle))
	}

	guardOK, guardDetail := checkHookHealth("guard", root)
	if guardOK {
		fmt.Println(statusLine("guard", iconOK, guardDetail, okStyle))
	} else {
		fmt.Println(statusLine("guard", iconFail, guardDetail, errStyle))
	}

	checkOK, checkDetail := checkHookHealth("check", root)
	if checkOK {
		fmt.Println(statusLine("check", iconOK, checkDetail, okStyle))
	} else {
		fmt.Println(statusLine("check", iconFail, checkDetail, errStyle))
	}

	if showSupport {
		fmt.Println()
		fmt.Println(support.StyledMessage())
	}

	return nil
}

// checkHookHealth runs a hook command as a subprocess and reports if it works.
func checkHookHealth(hook, root string) (ok bool, detail string) {
	binary, err := exec.LookPath("claudeignore")
	if err != nil {
		return false, "binary not in PATH"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, hook) //nolint:gosec // binary path comes from LookPath
	cmd.Dir = root

	if hook == "guard" {
		// Pipe a harmless test input — a path that won't match any deny entry
		testInput := `{"tool_name":"Read","tool_input":{"file_path":"__claudeignore_healthcheck__"}}`
		cmd.Stdin = bytes.NewBufferString(testInput)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return false, "timed out (5s)"
	}

	if err != nil {
		exitErr, isExit := err.(*exec.ExitError)
		if isExit && hook == "guard" && exitErr.ExitCode() == 2 {
			// Exit 2 = blocked, means guard is working
			return true, "ok (test path was blocked)"
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return false, fmt.Sprintf("exit %d — %s", exitErr.ExitCode(), errMsg)
	}

	return true, "ok"
}
