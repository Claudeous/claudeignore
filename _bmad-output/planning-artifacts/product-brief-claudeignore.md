---
title: "Product Brief: claudeignore"
status: "draft"
created: "2026-04-08"
updated: "2026-04-08"
scope: "short-term (alpha → stable)"
inputs:
  - README.md
  - CLAUDE.md
  - go.mod
  - .github/workflows/ci.yml
  - .github/workflows/release.yml
  - .goreleaser.yml
reviews:
  - skeptic (code-level security audit)
  - opportunity (strategic positioning)
  - dx-friction (developer experience)
---

# Product Brief: claudeignore

## Executive Summary

Claude Code without claudeignore is like committing without `.gitignore` — your secrets are exposed by default. Every developer using Claude Code today faces a silent risk: the AI can read `.env` files, API keys, credentials, and any sensitive file in the project directory. Unlike Cursor (`.cursorignore`), Windsurf (`.windsurfignore`), or GitHub Copilot (organization-level content exclusion), Claude Code has no built-in mechanism to respect `.gitignore` rules or block AI access to sensitive files.

**claudeignore** is an open-source Go CLI that bridges this gap. It syncs `.gitignore` rules to Claude Code's sandbox, enforcing layered protection — OS-level filesystem blocking, runtime tool interception, and drift detection — so developers can use Claude Code without worrying about accidental secret exposure. It never blocks your workflow: any error defaults to allow. It's free, installs in one command, and works with existing Git workflows.

The short-term goal: fix known security gaps, make claudeignore reliable, well-tested, and frictionless across macOS, Linux, and Windows — so it becomes the obvious answer when any Claude Code user asks "how do I protect my secrets?"

## The Problem

Claude Code operates with broad file system access. When a developer runs Claude Code in a project containing `.env` files, API keys, database credentials, or proprietary configuration, the AI can — and routinely does — read those files through any of its tools (Read, Bash, Grep, Glob, Edit). There is no native `.gitignore` awareness.

**Today's workarounds are inadequate:**
- **Manual `denyRead` configuration** — requires enumerating every sensitive path in `settings.json`; tedious, error-prone, breaks when `.gitignore` changes
- **Trusting the AI not to read secrets** — no enforcement; one careless prompt can expose credentials
- **Not using Claude Code on sensitive projects** — abandons productivity gains entirely

The problem intensifies with teams: each developer must manually configure protections, there's no shared policy, and new team members start unprotected. In Docker sandbox environments (CI/CD, automated workflows), autonomous agents run with `bypassPermissions` — no permission prompts, no human oversight, and broad filesystem access. This is the highest-risk scenario: a single misconfigured agent can silently read every credential in the project.

## The Solution

claudeignore provides automated, layered protection that derives its deny list from what developers already maintain — their `.gitignore`:

| Layer | What it protects | When it applies |
|-------|-----------------|-----------------|
| **Sandbox `denyRead`** | Bash, spawned processes | OS-level, loaded at startup |
| **PreToolUse guard hook** | Read, Write, Edit, Grep, Glob | Runtime, every tool call |
| **UserPromptSubmit check hook** | Drift detection | Every prompt, alerts when out of sync |

Setup is a single command (`claudeignore init`), with an interactive TUI that guides through mode selection and file configuration. Two operating modes: **Gitignore mode** (default) derives the deny list from `.gitignore`, while **Manual mode** gives full control via `.claude.ignore`.

For teams, project-scope hooks create a built-in viral loop: when a teammate clones a protected repo, they're immediately told they need claudeignore. Shared config files (`.claude.ignore`, `.claude.unignore`) commit to Git for consistent team-wide rules.

## What Makes This Different

- **Zero-config for most users** — leverages existing `.gitignore` rather than requiring a separate ignore file; Cursor and Windsurf both require maintaining a second ignore file
- **Layered protection** — OS-level sandbox + runtime hook + drift detection. Each layer covers different tool categories with explicit coverage boundaries
- **Git-native pattern resolution** — delegates to `git check-ignore`, guaranteeing exact parity with Git's own behavior including negation patterns, nested `.gitignore` files, and submodule boundaries
- **Fail-open by design** — never blocks the developer on tool errors; any guard failure defaults to allow. This directly addresses the #1 objection to security tooling: "it will break my flow"
- **Team-aware with viral onboarding** — project-scope hooks automatically warn unprotected teammates on clone, creating organic adoption pressure without requiring enforcement
- **Docker sandbox ready** — supports autonomous agent workflows where `bypassPermissions` mode makes protection essential, not optional

## Who This Serves

**Primary: Any developer using Claude Code** — from solo developers with side projects containing API keys, to team leads managing shared repositories with sensitive configuration. The common thread: they use Git, they have files they don't want AI to access, and they want protection without friction.

**Secondary: Teams and organizations** — engineering leads who need a standardized way to enforce AI file-access policies across their team, with shared configuration committed to the repository.

## Success Criteria (Short-Term)

| Metric | Target | Measurement |
|--------|--------|-------------|
| GitHub stars | Growing trend | GitHub Insights |
| Homebrew/Scoop installs | Measurable adoption | Homebrew analytics, download counts |
| Critical/security bugs | Zero open | GitHub Issues |
| Test coverage (overall) | ≥50% line coverage | `go test -coverprofile` |
| Test coverage (hooks) | ≥70% line coverage | `go test -coverprofile` (hooks pkg) |
| Platform verification | macOS + Linux + Windows | CI matrix green |
| Security scanning | Zero vulnerabilities | govulncheck + CodeQL |

## Scope

### In Scope (Short-Term)

**Security fixes (P0):**
- Fix Glob tool bypass — guard.go does not handle the `pattern` field, all Glob calls pass unprotected
- Fix Grep guard bypass — glob patterns like `**/*.go` that don't intersect deny list are not blocked
- Fix hooks installation to merge (not replace) the `hooks` key in `~/.claude/settings.json` — current behavior destroys other tools' hooks
- Fix Windows-incompatible code — `ps -p`, `/dev/stdin`, PPID walk are POSIX-only; Windows binaries ship but silently fail

**Quality & testing:**
- Increase test coverage to ≥50% overall, ≥70% on hooks — priority on `guard.go` (Glob/Grep paths), `check.go`, and command-level smoke tests
- Cross-platform CI testing — add Linux and Windows to the test matrix
- Security audit of guard/hook layer edge cases

**Release hygiene:**
- Merge Docker sandbox support (PR #21)
- Version alignment — sync `main.go` version with Git tags
- Create and maintain CHANGELOG.md

**UX polish:**
- Add post-sync confirmation showing protection status and mode used
- Improve error messages when no git repo found (suggest manual mode)
- Document Windows degradation explicitly in README

### Out of Scope (Short-Term)
- Support for AI tools other than Claude Code
- Audit logging, compliance reporting, or `claudeignore verify` CI command
- Enterprise policy enforcement dashboards
- GUI or web interface
- Commercial features or paid tiers
- License change (GPLv3 stays for now)

## Vision

claudeignore becomes the **standard security layer** for Claude Code — the tool every developer installs alongside Claude Code itself, like a seatbelt you put on before driving. As Anthropic's ecosystem grows and agentic AI becomes the norm, claudeignore evolves from "protect your secrets" to "control what AI can access in your project."

If Anthropic ships native `.gitignore` support, claudeignore's defense-in-depth approach, team policy features, fine-grained unignore/ignore controls, and Docker sandbox support remain valuable as the power-user layer on top of native basics.

## Known Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Anthropic ships native gitignore support | Medium | Differentiate on unignore, team policy, Docker; position as complement |
| Fail-open silently disables protection when binary not in PATH | High | Add health check to `status` command; document clearly |
| `settings.local.json` can be overwritten by Claude Code | High | Detect tampering via hash comparison in check hook |
| Claude Code hook IPC protocol changes | Medium | Pin minimum supported Claude Code version; version guard in init |
| curl-pipe-sh install pattern for a security tool | Medium | Add checksum verification to install script |
