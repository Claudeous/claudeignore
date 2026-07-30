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

	t.Run("refuses to overwrite invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		original := `{"permissions": {"allow": ["Read"]},}` // trailing comma
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}

		err := InstallHooksToFile(path, UserHooksConfig())
		if err == nil {
			t.Fatal("expected an error for an unparseable settings file")
		}

		// The user's file must be left exactly as it was
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Errorf("file was modified:\n%s", data)
		}
	})

	t.Run("preserves sandbox settings written by the user", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		input := `{
  "sandbox": {
    "network": {"allowUnixSockets": ["/var/run/docker.sock"]},
    "filesystem": {"denyRead": [".env"]}
  }
}`
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}

		if err := InstallHooksToFile(path, UserHooksConfig()); err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		sandbox, ok := m["sandbox"].(map[string]interface{})
		if !ok {
			t.Fatal("sandbox key was dropped")
		}
		if sandbox["network"] == nil {
			t.Error("sandbox.network was dropped")
		}
		if sandbox["filesystem"] == nil {
			t.Error("sandbox.filesystem was dropped")
		}
	})
}

func TestInstallSandboxSettings(t *testing.T) {
	t.Run("preserves sibling sandbox keys", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		input := `{
  "env": {"FOO": "bar"},
  "sandbox": {
    "network": {"allow": [{"host": "example.com"}]},
    "filesystem": {"denyRead": [".env"], "allowWrite": ["/tmp/x"]}
  }
}`
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}

		if err := InstallSandboxSettings(path); err != nil {
			t.Fatalf("InstallSandboxSettings error: %v", err)
		}

		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		if m["env"] == nil {
			t.Error("env key was dropped")
		}
		sandbox, ok := m["sandbox"].(map[string]interface{})
		if !ok {
			t.Fatal("sandbox key was dropped")
		}
		if sandbox["network"] == nil {
			t.Error("sandbox.network was dropped")
		}
		if sandbox["enabled"] != true || sandbox["autoAllowBashIfSandboxed"] != true {
			t.Errorf("sandbox flags not applied: %v", sandbox)
		}
		fs, ok := sandbox["filesystem"].(map[string]interface{})
		if !ok {
			t.Fatal("sandbox.filesystem was dropped")
		}
		if fs["allowWrite"] == nil {
			t.Error("sandbox.filesystem.allowWrite was dropped")
		}
		if m["defaultMode"] != "bypassPermissions" {
			t.Errorf("defaultMode = %v, want bypassPermissions", m["defaultMode"])
		}
	})

	t.Run("refuses to overwrite invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		original := `{"sandbox": }`
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}

		if err := InstallSandboxSettings(path); err == nil {
			t.Fatal("expected an error for an unparseable settings file")
		}
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Errorf("file was modified:\n%s", data)
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
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("failed to seed settings file: %v", err)
		}

		err := InstallHooksToFile(path, UserHooksConfig())
		if err != nil {
			t.Fatalf("InstallHooksToFile error: %v", err)
		}

		raw, _ := os.ReadFile(path)
		var result map[string]interface{}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("failed to parse settings: %v", err)
		}

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
		if err := InstallHooksToFile(path, UserHooksConfig()); err != nil {
			t.Fatalf("first install failed: %v", err)
		}
		// Second install (should not duplicate)
		if err := InstallHooksToFile(path, UserHooksConfig()); err != nil {
			t.Fatalf("second install failed: %v", err)
		}

		raw, _ := os.ReadFile(path)
		var result map[string]interface{}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("failed to parse settings: %v", err)
		}

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
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("failed to seed settings file: %v", err)
		}

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
