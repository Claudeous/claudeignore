package commands

import (
	"fmt"

	"github.com/claudeous/claudeignore/internal/support"
)

// Help prints the usage information.
func Help() {
	fmt.Println(`claudeignore — Sync gitignore rules to Claude Code sandbox

Modes:
  gitignore (default)   sandbox = git_ignored - .claude.unignore + .claude.ignore
  manual                sandbox = .claude.ignore only

Usage:
  claudeignore init              Interactive setup (choose mode, configure, install hooks)
  claudeignore view              View files currently blocked from Claude
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
	fmt.Println()
	fmt.Println(support.StyledMessage())
}
