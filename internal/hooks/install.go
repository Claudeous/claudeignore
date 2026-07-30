package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/claudeous/claudeignore/internal/config"
)

// UserHooksConfig returns the hook configuration for user-scope settings.
func UserHooksConfig() map[string]interface{} {
	return map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"matcher": "Read|Write|Edit|Grep|Glob|NotebookEdit",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "claudeignore guard",
					},
				},
			},
		},
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "claudeignore check",
					},
				},
			},
		},
	}
}

// CheckInstallScript is the content of the project hook script
// that warns teammates who don't have claudeignore installed.
const CheckInstallScript = `#!/bin/sh
which claudeignore >/dev/null 2>&1 && exit 0
echo '{"continue":true,"suppressOutput":false,"systemMessage":"\u26a0\ufe0f This project uses claudeignore to protect sensitive files. Install it: brew tap claudeous/tools \u0026\u0026 brew install claudeignore — https://github.com/Claudeous/claudeignore"}'
`

// CheckInstallScriptPath returns the path to the check-install script.
func CheckInstallScriptPath(root string) string {
	return filepath.Join(root, ".claude", "claudeignore", "check-install.sh")
}

// WriteCheckInstallScript creates the check-install script on disk.
func WriteCheckInstallScript(root string) error {
	scriptPath := CheckInstallScriptPath(root)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0750); err != nil {
		return fmt.Errorf("cannot create .claude/claudeignore directory: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(CheckInstallScript), 0750); err != nil { //nolint:gosec // shell script must be executable
		return fmt.Errorf("cannot write check-install script: %w", err)
	}
	return nil
}

// ProjectHooksConfig returns the hook configuration for project-scope settings.
func ProjectHooksConfig() map[string]interface{} {
	return map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type": "command",
						// Anchor to CLAUDE_PROJECT_DIR so the hook still resolves
						// after Claude Code changes cwd (via /cd or the Bash tool's
						// persistent shell). A bare relative path would fail with
						// "No such file or directory" from the new cwd.
						"command": `"$CLAUDE_PROJECT_DIR/.claude/claudeignore/check-install.sh"`,
					},
				},
			},
		},
	}
}

// InstallHooksToFile writes hook configuration to a settings file, merging
// claudeignore hooks with any existing hooks from other tools. Every other
// setting in the file is preserved; a file that cannot be parsed is reported
// rather than overwritten.
func InstallHooksToFile(path string, hooks map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", path, err)
	}

	settings, err := config.LoadOrCreateSettings(path)
	if err != nil {
		return err
	}

	existing, _ := settings.Get("hooks")
	if err := settings.Set("hooks", mergeHooks(existing, hooks)); err != nil {
		return err
	}

	return config.SaveSettings(path, settings)
}

// mergeHooks merges claudeignore hooks into existing hooks, preserving
// non-claudeignore entries from other tools.
func mergeHooks(existing interface{}, ours map[string]interface{}) map[string]interface{} {
	existingMap, ok := existing.(map[string]interface{})
	if !ok {
		return ours
	}

	merged := make(map[string]interface{}, len(existingMap))
	// Copy all existing hook types
	for k, v := range existingMap {
		merged[k] = v
	}
	// Merge our hook types
	for hookType, ourEntries := range ours {
		ourArr, ok := ourEntries.([]interface{})
		if !ok {
			merged[hookType] = ourEntries
			continue
		}
		existArr, ok := merged[hookType].([]interface{})
		if !ok {
			merged[hookType] = ourEntries
			continue
		}
		merged[hookType] = mergeHookEntries(existArr, ourArr)
	}
	return merged
}

// mergeHookEntries merges claudeignore entries into an existing hook type array.
// Claudeignore entries are identified by having a command containing "claudeignore".
// Existing claudeignore entries are replaced; non-claudeignore entries are preserved.
func mergeHookEntries(existing, ours []interface{}) []interface{} {
	// Remove old claudeignore entries from existing
	var kept []interface{}
	for _, entry := range existing {
		if !isClaudeignoreEntry(entry) {
			kept = append(kept, entry)
		}
	}
	// Append our entries
	return append(kept, ours...)
}

// isClaudeignoreEntry checks if a hook entry belongs to claudeignore
// by looking for "claudeignore" in any hook command string.
func isClaudeignoreEntry(entry interface{}) bool {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	hooks, ok := m["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, h := range hooks {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		if strings.Contains(cmd, "claudeignore") {
			return true
		}
	}
	return false
}

// InitSbxScript is the content of the sandbox init script
// that configures Claude Code with bypass permissions + OS-level sandbox.
const InitSbxScript = `#!/bin/bash
# claudeignore — Docker Sandbox (sbx) init script
#
# Installs claudeignore, syncs rules, installs hooks, and configures
# Claude Code with bypass permissions + OS-level filesystem protection.
#
# Usage (as root, from the workspace root directory):
#   cd /path/to/your/workspace
#   sbx exec -u root <sandbox-name> bash $(pwd)/.claude/claudeignore/init-sbx.sh
#
# Run once after sandbox creation. The sandbox is persistent.

set -e

WORKSPACE="$(cd "$(dirname "$0")/../.." && pwd)"

# ── Install claudeignore ─────────────────────────────────────────────
echo "Installing claudeignore..."
curl -fsSL https://raw.githubusercontent.com/Claudeous/claudeignore/main/install.sh | sh

# ── Configure (as agent user) ───────────────────────────────────────
su agent -c "cd $WORKSPACE && claudeignore sync && claudeignore install-hook && claudeignore configure-sbx"

echo ""
echo "Done! claudeignore + sandbox configured."
echo "  - Guard hook: protects Read/Write/Edit/Grep/NotebookEdit"
echo "  - Sandbox denyRead: protects Bash (cat, grep, scripts, etc.)"
echo ""
echo "Verify with: claudeignore status"
`

// InitSbxScriptPath returns the path to the init-sbx script.
func InitSbxScriptPath(root string) string {
	return filepath.Join(root, ".claude", "claudeignore", "init-sbx.sh")
}

// WriteInitSbxScript creates the init-sbx script on disk.
func WriteInitSbxScript(root string) error {
	scriptPath := InitSbxScriptPath(root)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0750); err != nil {
		return fmt.Errorf("cannot create .claude/claudeignore directory: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(InitSbxScript), 0750); err != nil { //nolint:gosec // shell script must be executable
		return fmt.Errorf("cannot write init-sbx script: %w", err)
	}
	return nil
}

// InstallSandboxSettings merges sandbox settings into a settings file,
// preserving other keys — including sibling keys under sandbox such as
// network rules or filesystem.denyRead.
func InstallSandboxSettings(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", path, err)
	}

	settings, err := config.LoadOrCreateSettings(path)
	if err != nil {
		return err
	}

	updates := []struct {
		value interface{}
		path  []string
	}{
		{"bypassPermissions", []string{"defaultMode"}},
		{true, []string{"skipDangerousModePermissionPrompt"}},
		{true, []string{"sandbox", "enabled"}},
		{true, []string{"sandbox", "autoAllowBashIfSandboxed"}},
	}
	for _, u := range updates {
		if err := settings.SetPath(u.value, u.path...); err != nil {
			return err
		}
	}

	return config.SaveSettings(path, settings)
}

// OutputHookMessage prints a JSON hook message to stdout.
func OutputHookMessage(message string) {
	result := map[string]interface{}{
		"continue":       true,
		"suppressOutput": false,
		"systemMessage":  message,
	}
	out, _ := json.Marshal(result)
	fmt.Println(string(out))
}
