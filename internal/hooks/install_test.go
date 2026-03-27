package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooksToFile(t *testing.T) {
	t.Run("creates new file with hooks", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".claude", "settings.json")

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("invalid JSON output: %v", err)
		}

		if m["hooks"] == nil {
			t.Error("hooks key not found")
		}
	})

	t.Run("preserves existing keys", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]}}`), 0644)

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		json.Unmarshal(data, &m)

		if m["permissions"] == nil {
			t.Error("existing 'permissions' key was not preserved")
		}
		if m["hooks"] == nil {
			t.Error("hooks key not added")
		}
	})

	t.Run("overwrites invalid JSON gracefully", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		os.WriteFile(path, []byte(`not valid json`), 0644)

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		// Should have created valid JSON
		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("output should be valid JSON: %v", err)
		}
	})
}

func TestUserHooksConfig(t *testing.T) {
	hooks := UserHooksConfig()

	if hooks["PreToolUse"] == nil {
		t.Error("missing PreToolUse hook")
	}
	if hooks["UserPromptSubmit"] == nil {
		t.Error("missing UserPromptSubmit hook")
	}
}

func TestProjectHooksConfig(t *testing.T) {
	hooks := ProjectHooksConfig()

	if hooks["UserPromptSubmit"] == nil {
		t.Error("missing UserPromptSubmit hook")
	}
	// Project config should NOT have PreToolUse (that's user scope only)
	if hooks["PreToolUse"] != nil {
		t.Error("project config should not have PreToolUse")
	}
}
