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

### macOS (Homebrew)

```bash
brew tap claudeous/tools
brew install claudeignore
```

### macOS / Linux (curl)

```bash
curl -fsSL https://raw.githubusercontent.com/Claudeous/claudeignore/main/install.sh | sh
```

Installs to `/usr/local/bin` (may prompt for sudo).

### Windows (Scoop)

```powershell
scoop bucket add claudeous https://github.com/Claudeous/scoop-tools
scoop install claudeignore
```

### Windows (manual)

Download the latest `.zip` from [Releases](https://github.com/Claudeous/claudeignore/releases), extract `claudeignore.exe`, and add its folder to your `PATH`.

### All platforms (Go)

```bash
go install github.com/Claudeous/claudeignore@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin` (must be in your `PATH`).

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
| `claudeignore install-hook`   | Install hooks (user + project scope)    |
| `claudeignore status`         | Show current state                      |

## How it works

1. `init` lets you choose a mode (gitignore or manual), then configures rules and installs hooks.
2. `sync` computes the deny list based on the chosen mode, writes to `settings.local.json`.
3. `guard` hook intercepts Read/Write/Edit/Grep/Glob and blocks access to denied paths.
4. `check` hook alerts on every prompt if rules are out of sync or restart is needed.
5. `install-hook` installs hooks in two scopes: **user** (`~/.claude/settings.json`, direct commands) and **project** (`.claude/settings.json`, safe wrapper with `which` check for teammates without the binary).

## Change detection

The `check` hook detects two situations:

- **Out of sync**: rules changed on disk (new files, edited config)
- **Restart pending**: `sync` was run but Claude Code hasn't been restarted (sandbox needs reload for Bash protection)

Restart detection works by comparing the sync timestamp with the Claude Code process start time (via PPID walk).

## FAQ

**Why does each dev need to run `sync` locally?**

The source rules (`.gitignore`, `.claude.ignore`, `.claude.unignore`) are committed and shared via git. But the generated deny list in `settings.local.json` depends on what files actually exist on disk, which varies per machine. Each dev runs `claudeignore sync` to resolve rules against their local state — similar to how `node_modules` is local but `package-lock.json` is shared.

**What happens for teammates who don't have claudeignore?**

The project hook (`.claude/settings.json`) runs a check script that detects if the binary is missing and shows an install reminder. No protection is enforced without the binary — the hook is advisory only.

**Why is a restart required after sync?**

The sandbox `denyRead` list is loaded once when Claude Code starts. After `sync` updates it, Bash-level protection only takes effect after restart. The `guard` hook protects Read/Write/Edit/Grep/Glob immediately — only Bash access requires the restart.

**Does the guard block every access attempt?**

The guard fails open by design: if it can't determine the repo root, read settings, or parse input, it allows the request. This prevents the tool from ever locking a user out due to a bug or misconfiguration.

**How do I know if I'm protected?**

Run `claudeignore status`. It shows the current mode, sync state, number of denied entries, and whether hooks are installed. If everything says "up to date" and hooks show "user + project", you're fully protected.

## Support

claudeignore is free and open source, built and maintained by [Claudeous](https://github.com/Claudeous) — an independent developer creating tools for the Claude Code ecosystem, making AI-assisted development safer, one tool at a time.

If this tool is useful to you, consider supporting development:

- [GitHub Sponsors](https://github.com/sponsors/Claudeous)

Your sponsorship helps maintain claudeignore, respond to issues, and build more tools that make AI-assisted development safer and smoother.

## Requirements

- `git`
