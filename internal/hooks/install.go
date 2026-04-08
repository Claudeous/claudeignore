package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
						"type":    "command",
						"command": ".claude/claudeignore/check-install.sh",
					},
				},
			},
		},
	}
}

// InstallHooksToFile writes hook configuration to a settings file, preserving other keys.
func InstallHooksToFile(path string, hooks map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", path, err)
	}

	var settings map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	}

	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
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

// InstallSandboxSettings merges sandbox settings into a settings file, preserving other keys.
func InstallSandboxSettings(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", path, err)
	}

	var settings map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	}

	settings["defaultMode"] = "bypassPermissions"
	settings["skipDangerousModePermissionPrompt"] = true

	sandbox, ok := settings["sandbox"].(map[string]interface{})
	if !ok {
		sandbox = make(map[string]interface{})
	}
	sandbox["enabled"] = true
	sandbox["autoAllowBashIfSandboxed"] = true
	settings["sandbox"] = sandbox

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
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
