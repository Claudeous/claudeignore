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
				"matcher": "Read|Write|Edit|Grep|Glob",
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

// ProjectHooksConfig returns the hook configuration for project-scope settings.
func ProjectHooksConfig() map[string]interface{} {
	checkScript := `which claudeignore >/dev/null 2>&1 && exit 0; echo '{"continue":true,"suppressOutput":false,"systemMessage":"` +
		`\u26a0\ufe0f This project uses claudeignore to protect sensitive files. ` +
		`Install it: brew tap claudeous/tools \u0026\u0026 brew install claudeignore ` +
		`— https://github.com/Claudeous/claudeignore"}'`

	return map[string]interface{}{
		"UserPromptSubmit": []interface{}{
			map[string]interface{}{
				"matcher": "",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": checkScript,
					},
				},
			},
		},
	}
}

// InstallHooksToFile writes hook configuration to a settings file, preserving other keys.
func InstallHooksToFile(path string, hooks map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
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
	return os.WriteFile(path, append(out, '\n'), 0644)
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
