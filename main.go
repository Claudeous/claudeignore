// Binary entry point — delegates to cmd/claudeignore.
// This file exists so that `go build .` and `go run .` continue to work
// from the repository root. The actual logic lives in internal/ packages.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/commands"
	"github.com/claudeous/claudeignore/internal/config"
	"github.com/claudeous/claudeignore/internal/git"
	"github.com/claudeous/claudeignore/internal/hooks"
	"github.com/claudeous/claudeignore/internal/support"
	"github.com/claudeous/claudeignore/internal/tui"
)

var version = "0.0.4-alpha"

var menuItems = []tui.MenuItem{
	{Name: "init", Desc: "Interactive TUI to select what Claude can read"},
	{Name: "view", Desc: "View files currently blocked from Claude"},
	{Name: "sync", Desc: "Apply current rules to sandbox"},
	{Name: "check", Desc: "Check if rules changed (for hooks)"},
	{Name: "guard", Desc: "Block tool access to denied files (for hooks)"},
	{Name: "status", Desc: "Show current state"},
	{Name: "install-hook", Desc: "Install all hooks in .claude/settings.json"},
	{Name: "help", Desc: "Show help"},
	{Name: "version", Desc: "Show version"},
	{Name: "\U0001F49C support", Desc: "Open sponsor page in browser"},
}

func main() {
	if len(os.Args) > 1 {
		if err := runCommand(os.Args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	// No argument → interactive menu
	root := resolveRootBestEffort()

	if root != "" {
		if err := commands.Status(root, version, false); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		fmt.Println()
	}

	m := tui.NewMenuModel(menuItems, version)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		os.Exit(1)
	}
	final := result.(tui.MenuModel)
	if final.Chosen != "" {
		fmt.Println()
		if final.Chosen == "\U0001F49C support" {
			if err := support.OpenBrowser(); err != nil {
				fmt.Fprintln(os.Stderr, "Could not open browser:", err)
			} else {
				fmt.Println("Opening sponsor page...")
			}
		} else if err := runCommand(final.Chosen); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	}
}

// resolveRootBestEffort returns the project root, trying git first then cwd with manual mode state.
// Returns empty string if no root can be determined.
func resolveRootBestEffort() string {
	root, err := git.RepoRoot()
	if err == nil {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	state := config.LoadState(cwd)
	if state.Mode == "manual" {
		return cwd
	}
	return ""
}

// resolveRoot returns the project root for a given command.
// For init, always falls back to cwd (user may set up manual mode).
// For other commands, falls back to cwd only if manual mode is already configured.
func resolveRoot(cmd string) (string, error) {
	root, err := git.RepoRoot()
	if err == nil {
		return root, nil
	}
	gitErr := err

	cwd, err := os.Getwd()
	if err != nil {
		return "", gitErr
	}

	// For init, always allow cwd — user might choose manual mode
	if cmd == "init" {
		return cwd, nil
	}

	// For other commands, allow cwd if manual mode is configured
	state := config.LoadState(cwd)
	if state.Mode == "manual" {
		return cwd, nil
	}

	return "", gitErr
}

func runCommand(cmd string) error {
	// When invoked as a Claude Code hook, the process cwd may not be the
	// project root: Claude can change directory via /cd or via the Bash
	// tool's persistent shell, and any subsequent hook is launched from
	// that new directory. If the new cwd happens to be a sub-repo, plain
	// `git rev-parse --show-toplevel` resolves to the wrong root and the
	// hook silently fails to find .claude/claudeignore/.
	//
	// Claude Code always exports CLAUDE_PROJECT_DIR pointing at the
	// original project directory where the user launched claude. Honor it
	// for hook commands so all downstream resolution (git root, state
	// file, log file) is anchored to the right place. Best-effort: if the
	// chdir fails we fall back to the inherited cwd.
	if cmd == "check" || cmd == "guard" {
		if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
			_ = os.Chdir(dir)
		}
	}

	needsRoot := cmd != "help" && cmd != "--help" && cmd != "-h" && cmd != "version" && cmd != "support" && cmd != "configure-sbx"
	var root string
	if needsRoot {
		var err error
		root, err = resolveRoot(cmd)
		if err != nil {
			if cmd == "guard" || cmd == "check" {
				if errors.Is(err, git.ErrNotGitRepo) {
					hooks.WarnNotGitRepoOnce()
				} else {
					hooks.OutputHookMessage(fmt.Sprintf("claudeignore: cannot find repo root: %v", err))
				}
				return nil
			}
			return err
		}
	}

	switch cmd {
	case "init":
		return commands.Init(root)
	case "view":
		return commands.View(root)
	case "sync":
		dryRun := false
		if len(os.Args) > 2 {
			for _, arg := range os.Args[2:] {
				if arg == "--dry-run" {
					dryRun = true
				}
			}
		}
		return commands.Sync(root, dryRun)
	case "check":
		hooks.HookLog(root, "check", "started")
		result, err := hooks.Check(root)
		if err != nil {
			hooks.HookLogError(root, "check", err)
			hooks.OutputHookMessage(fmt.Sprintf("claudeignore check error: %v", err))
			return nil
		}
		if result != nil {
			msg := hooks.FormatCheckMessage(result)
			hooks.HookLog(root, "check", fmt.Sprintf("result: %s", msg))
			hooks.OutputHookMessage(msg)
		} else {
			hooks.HookLog(root, "check", "up to date")
		}
		return nil
	case "guard":
		guardResult, err := hooks.Guard(root)
		if err != nil {
			// Guard MUST fail open (see CLAUDE.md gotchas). We report the
			// error via the standard hook message envelope (stdout JSON),
			// never via a deny response, and always return nil so the process
			// exits 0. Using OutputHookMessage (which uses json.Marshal)
			// ensures the error text is properly escaped.
			hooks.HookLogError(root, "guard", err)
			hooks.OutputHookMessage(fmt.Sprintf("claudeignore guard error: %v", err))
			return nil
		}
		if guardResult.Blocked {
			hooks.HookLog(root, "guard", fmt.Sprintf("blocked: %s", guardResult.Reason))
			fmt.Fprintln(os.Stderr, string(hooks.GuardDenyResponse(guardResult.Reason)))
			os.Exit(2)
		}
		if guardResult.UpdatedInput != nil {
			hooks.HookLog(root, "guard", "injected exclusion glob")
			fmt.Println(string(guardResult.UpdatedInput))
		}
		return nil
	case "install-hook":
		return commands.InstallHook(root)
	case "configure-sbx":
		if os.Getenv("IS_SANDBOX") != "1" {
			return fmt.Errorf("configure-sbx is intended for Docker sandboxes only (set IS_SANDBOX=1 to override)")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot find home directory: %w", err)
		}
		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := hooks.InstallSandboxSettings(settingsPath); err != nil {
			return fmt.Errorf("error configuring sandbox settings: %w", err)
		}
		fmt.Println("Sandbox settings configured in ~/.claude/settings.json")
		fmt.Println("  - defaultMode: bypassPermissions")
		fmt.Println("  - sandbox.enabled: true")
		fmt.Println("  - autoAllowBashIfSandboxed: true")
		return nil
	case "status":
		return commands.Status(root, version, true)
	case "version":
		fmt.Printf("claudeignore %s\n", version)
		return nil
	case "support":
		if err := support.OpenBrowser(); err != nil {
			return fmt.Errorf("could not open browser: %w", err)
		}
		fmt.Println("Opening sponsor page...")
		return nil
	case "help", "--help", "-h":
		commands.Help()
		return nil
	default:
		return fmt.Errorf("unknown command: %s\nRun 'claudeignore help' for available commands", cmd)
	}
}
