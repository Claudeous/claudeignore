# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**claudeignore** — a Go CLI that syncs `.gitignore` rules to Claude Code's sandbox, blocking AI file access to secrets/`.env`/etc. via all tools (Read, Bash, Grep, etc.).

## Build & Run

```bash
go build -o claudeignore .       # build binary
go run . <cmd>                   # run without building (e.g. go run . status)
./claudeignore                   # interactive menu
./claudeignore init              # setup wizard
./claudeignore sync --dry-run    # preview deny list
make test                        # run tests with race detector
make vet                         # run go vet
make cover                       # generate coverage report
```

### Release

```bash
git tag v0.x.x && git push origin v0.x.x   # triggers GoReleaser via GitHub Actions
```

## Architecture

```
main.go                     # Entry point, CLI routing
internal/
  git/git.go                # Git helpers (repoRoot, ignored paths)
  config/config.go          # Settings types, file helpers, path sets
  config/state.go           # State persistence, hash computation
  hooks/guard.go            # PreToolUse hook (path blocking)
  hooks/check.go            # UserPromptSubmit hook (sync detection)
  hooks/install.go          # Hook config generation, installation
  tui/styles.go             # Shared lipgloss styles
  tui/filepicker.go         # File selection TUI
  tui/modeselector.go       # Mode selection TUI
  tui/menu.go               # Main menu TUI
  commands/                  # CLI command implementations
```

### Three protection layers

1. **Sandbox `denyRead`** — OS-level filesystem block (Bash/spawned processes), written to `.claude/settings.local.json`
2. **PreToolUse hook (`guard`)** — blocks Read/Write/Edit/Grep/Glob by matching paths against the deny list; exit code 2 + JSON on stderr to block
3. **UserPromptSubmit hook (`check`)** — hash-based change detection, alerts when rules are out of sync or restart is needed

### Two modes

- **Gitignore mode** (default): `denyList = .gitignore - .claude.unignore + .claude.ignore`
- **Manual mode**: `denyList = .claude.ignore` only

### Hook scopes

Hooks are installed in two places:
- **User scope** (`~/.claude/settings.json`) — actual guard + check commands
- **Project scope** (`.claude/settings.json`) — warns teammates who don't have the binary

### State tracking

`.claude/.claude.ignore.state.json` stores mode, SHA-256 hash of config files, sync timestamp, and newly denied files. Restart detection compares sync timestamp against Claude Code process start time (via PPID walk).

### Key patterns

- TUI uses charmbracelet stack (bubbletea, bubbles, lipgloss) for mode selection, file selection, and main menu
- Git integration via `git status --ignored=traditional --porcelain` and `git check-ignore`
- Hook IPC is JSON over stdin/stdout/stderr
- Path blocking uses prefix matching (denying `dir/` blocks `dir/subfile`)

## Config files

| File | Purpose |
|------|---------|
| `.claude.unignore` | Paths from gitignore that Claude CAN read |
| `.claude.ignore` | Extra paths to block beyond gitignore |
| `.claude/settings.local.json` | Generated sandbox deny list |
| `.claude/settings.json` | Project-scope hooks (install warning for teammates) |
| `.claude/.claude.ignore.state.json` | Sync state (hash, timestamp, mode) |
| `~/.claude/settings.json` | User-scope hooks (actual guard + check commands) |

## Gotchas

- **Guard fails open**: any error in `guard` (bad JSON, missing settings, no repo root) exits 0 (allow). This is intentional — never block the user on tool errors.
- **Guard reads stdin**: when testing `guard` manually, pipe hook JSON via stdin: `echo '{"tool_name":"Read","tool_input":{"file_path":".env"}}' | go run . guard`
- **Restart required after sync**: sandbox `denyRead` is loaded at Claude Code startup. After `sync`, the `check` hook will remind to restart until the process is newer than the sync timestamp.
- **Patterns are resolved by git**: `.claude.ignore` and `.claude.unignore` use gitignore syntax and are resolved via `git check-ignore`, not by the Go code directly.