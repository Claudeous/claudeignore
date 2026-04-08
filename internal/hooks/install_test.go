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
		if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]}}`), 0600); err != nil {
			t.Fatal(err)
		}

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}

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
		if err := os.WriteFile(path, []byte(`not valid json`), 0600); err != nil {
			t.Fatal(err)
		}

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

func TestInstallHooksToFile_PreservesOtherHooks(t *testing.T) {
	t.Run("existing hooks from other tools are preserved", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")

		// Pre-existing hooks from another tool
		existing := map[string]interface{}{
			"hooks": map[string]interface{}{
				"PreToolUse": []interface{}{
					map[string]interface{}{
						"matcher": "Bash",
						"hooks": []interface{}{
							map[string]interface{}{
								"type":    "command",
								"command": "my-other-tool lint",
							},
						},
					},
				},
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"matcher": "",
						"hooks": []interface{}{
							map[string]interface{}{
								"type":    "command",
								"command": "my-logger log",
							},
						},
					},
				},
			},
		}
		data, _ := json.Marshal(existing)
		os.WriteFile(path, data, 0600)

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		raw, _ := os.ReadFile(path)
		var result map[string]interface{}
		json.Unmarshal(raw, &result)

		hooks := result["hooks"].(map[string]interface{})

		// PostToolUse should be preserved (claudeignore doesn't use it)
		if hooks["PostToolUse"] == nil {
			t.Error("PostToolUse from other tool was destroyed")
		}

		// PreToolUse should have both: other tool's Bash hook + claudeignore's hook
		preToolUse := hooks["PreToolUse"].([]interface{})
		if len(preToolUse) < 2 {
			t.Errorf("expected at least 2 PreToolUse entries, got %d", len(preToolUse))
		}

		// Check the other tool's hook is still there
		found := false
		for _, entry := range preToolUse {
			m := entry.(map[string]interface{})
			if m["matcher"] == "Bash" {
				found = true
			}
		}
		if !found {
			t.Error("other tool's Bash hook was not preserved")
		}
	})

	t.Run("reinstall updates claudeignore hooks", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")

		// First install
		InstallHooksToFile(path, UserHooksConfig())
		// Second install (should not duplicate)
		InstallHooksToFile(path, UserHooksConfig())

		raw, _ := os.ReadFile(path)
		var result map[string]interface{}
		json.Unmarshal(raw, &result)

		hooks := result["hooks"].(map[string]interface{})
		preToolUse := hooks["PreToolUse"].([]interface{})

		// Should have exactly 1 PreToolUse entry, not 2
		if len(preToolUse) != 1 {
			t.Errorf("expected 1 PreToolUse entry after reinstall, got %d", len(preToolUse))
		}
	})

	t.Run("corrupted hooks key handled gracefully", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")

		existing := map[string]interface{}{
			"hooks": "not-a-map",
		}
		data, _ := json.Marshal(existing)
		os.WriteFile(path, data, 0600)

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		raw, _ := os.ReadFile(path)
		var result map[string]interface{}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal("output should be valid JSON")
		}
		if result["hooks"] == nil {
			t.Error("hooks should exist after fixing corrupted value")
		}
	})
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
