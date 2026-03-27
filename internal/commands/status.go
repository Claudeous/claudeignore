package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/claudeous/claudeignore/internal/config"
)

// Status prints the current state of claudeignore.
func Status(root string, version string) error {
	fmt.Printf("claudeignore v%s\n", version)
	fmt.Printf("Project: %s\n\n", root)

	state := config.LoadState(root)
	mode := state.Mode
	if mode == "" {
		mode = "gitignore"
	}
	fmt.Printf("Mode: %s\n", mode)

	if mode != "manual" {
		notignore := config.ReadLines(filepath.Join(root, ".claude.unignore"))
		if notignore != nil {
			fmt.Printf(".claude.unignore: %d path(s) (allowed)\n", len(notignore))
		} else {
			fmt.Println(".claude.unignore: not found")
		}
	}

	extra := config.ReadLines(filepath.Join(root, ".claude.ignore"))
	if extra != nil {
		fmt.Printf(".claude.ignore: %d path(s) (extra deny)\n", len(extra))
	} else {
		fmt.Println(".claude.ignore: not found")
	}

	if state.Hash == "" {
		fmt.Println("Sync: never run")
	} else {
		current := config.ComputeHash(root, mode)
		if current == state.Hash {
			fmt.Println("Sync: up to date")
		} else {
			fmt.Println("Sync: out of date (run 'claudeignore sync')")
		}
	}

	settingsPath := filepath.Join(root, ".claude", "settings.local.json")
	if settings, err := config.LoadSettings(settingsPath); err == nil {
		denyList := settings.GetDenyList()
		fmt.Printf("Sandbox denyRead: %d entry(ies)\n", len(denyList))
	} else {
		fmt.Println("Sandbox: not configured")
	}

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

	return nil
}
