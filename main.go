// Binary entry point — delegates to cmd/claudeignore.
// This file exists so that `go build .` and `go run .` continue to work
// from the repository root. The actual logic lives in internal/ packages.
package main

import (
	"fmt"
	"os"

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
	needsRoot := cmd != "help" && cmd != "--help" && cmd != "-h" && cmd != "version" && cmd != "support"
	var root string
	if needsRoot {
		var err error
		root, err = resolveRoot(cmd)
		if err != nil {
			if cmd == "guard" || cmd == "check" {
				return nil // hooks fail open
			}
			return err
		}
	}

	switch cmd {
	case "init":
		return commands.Init(root)
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
		result, err := hooks.Check(root)
		if err != nil {
			return nil
		}
		if result != nil {
			msg := hooks.FormatCheckMessage(result)
			hooks.OutputHookMessage(msg)
		}
		return nil
	case "guard":
		guardResult, err := hooks.Guard(root)
		if err != nil {
			return nil
		}
		if guardResult.Blocked {
			fmt.Fprintln(os.Stderr, string(hooks.GuardDenyResponse(guardResult.Reason)))
			os.Exit(2)
		}
		if guardResult.UpdatedInput != nil {
			fmt.Println(string(guardResult.UpdatedInput))
		}
		return nil
	case "install-hook":
		return commands.InstallHook(root)
	case "status":
		return commands.Status(root, version, true)
	case "version":
		fmt.Printf("claudeignore v%s\n", version)
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
