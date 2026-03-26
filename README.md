# claudeignore

Sync gitignore rules to Claude Code sandbox, blocking file access via **all** tools (Read, Bash, Grep, etc.).

## Why?

Claude Code has no built-in way to block access to gitignored files (`.env`, secrets, etc.). This tool fills that gap by blocking **all** access paths:

- **Sandbox `denyRead`** — OS-level filesystem restriction that blocks any process spawned by Bash (`cat`, `grep`, scripts, etc.)
- **PreToolUse hook (`guard`)** — blocks built-in tools (Read, Write, Edit, Grep, Glob)
- **UserPromptSubmit hook (`check`)** — detects rule changes and reminds to sync & restart

## Modes

### Gitignore mode (default)

Derives the deny list from your `.gitignore`, with overrides:

$$\text{denyList} = \text{.gitignore} - \text{.claude.unignore} + \text{.claude.ignore}$$

| File               | Role                             | Example                          |
|--------------------|----------------------------------|----------------------------------|
| `.gitignore`       | Base deny list (from git)        | `.env`, `node_modules`, `vendor` |
| `.claude.unignore` | Subtract: Claude CAN read these  | `vendor`, `node_modules`         |
| `.claude.ignore`   | Add: extra deny beyond gitignore | `config/dev.secrets.php`         |

### Manual mode

Uses `.claude.ignore` only — no `.gitignore` dependency for the deny list:

$$\text{denyList} = \text{.claude.ignore}$$

Choose the mode during `claudeignore init`.

Both `.claude.ignore` and `.claude.unignore` use [gitignore pattern syntax](https://git-scm.com/docs/gitignore#_pattern_format) (`*`, `**`, `!`, etc.).

## Install

```bash
brew tap claudeous/tools
brew install claudeignore
```

Or with Go:

```bash
go install github.com/Claudeous/claudeignore@latest
```

## Setup on a project

```bash
claudeignore init     # Choose mode, configure, hooks auto-installed
# Restart Claude Code
```

That's it — `init` runs `sync` and `install-hook` automatically.

## Commands

| Command                       | Description                             |
|-------------------------------|-----------------------------------------|
| `claudeignore`                | Interactive menu with status            |
| `claudeignore init`           | Choose mode + configure + install hooks |
| `claudeignore sync`           | Apply rules to sandbox                  |
| `claudeignore sync --dry-run` | Preview deny list without writing       |
| `claudeignore check`          | Check for changes (used by hook)        |
| `claudeignore guard`          | Block tool access (used by hook)        |
| `claudeignore install-hook`   | Install hooks in project                |
| `claudeignore status`         | Show current state                      |

## How it works

1. `init` lets you choose a mode (gitignore or manual), then configures rules and installs hooks.
2. `sync` computes the deny list based on the chosen mode, writes to `settings.local.json`.
3. `guard` hook intercepts Read/Write/Edit/Grep/Glob and blocks access to denied paths.
4. `check` hook alerts on every prompt if rules are out of sync or restart is needed.
5. `install-hook` wires everything into `.claude/settings.json`.

## Change detection

The `check` hook detects two situations:

- **Out of sync**: rules changed on disk (new files, edited config)
- **Restart pending**: `sync` was run but Claude Code hasn't been restarted (sandbox needs reload for Bash protection)

Restart detection works by comparing the sync timestamp with the Claude Code process start time (via PPID walk).

## Requirements

- `git`
