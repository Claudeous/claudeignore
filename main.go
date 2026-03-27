// Binary entry point — delegates to cmd/claudeignore.
// This file exists so that `go build .` and `go run .` continue to work
// from the repository root. The actual logic lives in internal/ packages.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/claudeous/claudeignore/internal/commands"
	"github.com/claudeous/claudeignore/internal/git"
	"github.com/claudeous/claudeignore/internal/hooks"
	"github.com/claudeous/claudeignore/internal/tui"
)

const version = "0.0.1-alpha"

var menuItems = []tui.MenuItem{
	{Name: "init", Desc: "Interactive TUI to select what Claude can read"},
	{Name: "sync", Desc: "Apply current rules to sandbox"},
	{Name: "check", Desc: "Check if rules changed (for hooks)"},
	{Name: "guard", Desc: "Block tool access to denied files (for hooks)"},
	{Name: "status", Desc: "Show current state"},
	{Name: "install-hook", Desc: "Install all hooks in .claude/settings.json"},
	{Name: "help", Desc: "Show help"},
	{Name: "version", Desc: "Show version"},
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
	root, err := git.RepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if err := commands.Status(root, version); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println()

	m := tui.NewMenuModel(menuItems, version)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		os.Exit(1)
	}
	final := result.(tui.MenuModel)
	if final.Chosen != "" {
		fmt.Println()
		if err := runCommand(final.Chosen); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	}
}

func runCommand(cmd string) error {
	needsRoot := cmd != "help" && cmd != "--help" && cmd != "-h" && cmd != "version"
	var root string
	if needsRoot {
		var err error
		root, err = git.RepoRoot()
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
		blocked, reason, err := hooks.Guard(root)
		if err != nil {
			return nil
		}
		if blocked {
			fmt.Fprintln(os.Stderr, string(hooks.GuardDenyResponse(reason)))
			os.Exit(2)
		}
		return nil
	case "install-hook":
		return commands.InstallHook(root)
	case "status":
		return commands.Status(root, version)
	case "version":
		fmt.Printf("claudeignore v%s\n", version)
		return nil
	case "help", "--help", "-h":
		commands.Help()
		return nil
	default:
		return fmt.Errorf("unknown command: %s\nRun 'claudeignore help' for available commands", cmd)
	}
}
