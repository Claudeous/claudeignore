package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The settings file belongs to the user and to Claude Code; claudeignore only
// owns sandbox.filesystem.denyRead. Syncing must never drop anything else.
func TestUpdateSettingsFile_PreservesEverythingElse(t *testing.T) {
	input := `{
  "permissions": {
    "allow": ["Bash(ls:*)"]
  },
  "env": {
    "FOO": "bar"
  },
  "sandbox": {
    "enabled": true,
    "autoAllowBashIfSandboxed": true,
    "network": {
      "allowUnixSockets": ["/var/run/docker.sock"],
      "allow": [{"host": "example.com"}]
    },
    "filesystem": {
      "denyRead": ["stale.txt"],
      "allowWrite": ["/tmp/scratch"],
      "denyWrite": ["/etc"]
    }
  },
  "hooks": {
    "PreToolUse": [{"matcher": "Read", "hooks": [{"type": "command", "command": "other-tool"}]}]
  }
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}

	if err := UpdateSettingsFile(path, []string{".env", "secrets/"}); err != nil {
		t.Fatalf("UpdateSettingsFile error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}

	for _, key := range []string{"permissions", "env", "hooks"} {
		if m[key] == nil {
			t.Errorf("top-level key %q was dropped", key)
		}
	}

	sandbox, ok := m["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatalf("sandbox is not an object: %v", m["sandbox"])
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled was dropped")
	}
	if sandbox["autoAllowBashIfSandboxed"] != true {
		t.Error("sandbox.autoAllowBashIfSandboxed was dropped")
	}
	network, ok := sandbox["network"].(map[string]interface{})
	if !ok {
		t.Fatalf("sandbox.network was dropped: %v", sandbox["network"])
	}
	if network["allowUnixSockets"] == nil || network["allow"] == nil {
		t.Errorf("sandbox.network contents were dropped: %v", network)
	}

	fs, ok := sandbox["filesystem"].(map[string]interface{})
	if !ok {
		t.Fatalf("sandbox.filesystem was dropped: %v", sandbox["filesystem"])
	}
	if fs["allowWrite"] == nil {
		t.Error("sandbox.filesystem.allowWrite was dropped")
	}
	if fs["denyWrite"] == nil {
		t.Error("sandbox.filesystem.denyWrite was dropped")
	}

	// denyRead is the one key claudeignore owns: it is replaced, not merged.
	s, err := ParseSettings(data)
	if err != nil {
		t.Fatal(err)
	}
	deny := s.GetDenyList()
	if len(deny) != 2 || deny[0] != ".env" || deny[1] != "secrets/" {
		t.Errorf("GetDenyList() = %v, want [.env secrets/]", deny)
	}
}

func TestUpdateSettingsFile_PreservesKeyOrder(t *testing.T) {
	input := `{"zeta":1,"sandbox":{"network":{},"filesystem":{"allowWrite":[]}},"alpha":2}`

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSettingsFile(path, []string{".env"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	got := string(data)

	if idxZeta, idxAlpha := strings.Index(got, `"zeta"`), strings.Index(got, `"alpha"`); idxZeta > idxAlpha {
		t.Errorf("top-level key order was not preserved:\n%s", got)
	}
	if idxNet, idxFS := strings.Index(got, `"network"`), strings.Index(got, `"filesystem"`); idxNet > idxFS {
		t.Errorf("sandbox key order was not preserved:\n%s", got)
	}
	if idxAllow, idxDeny := strings.Index(got, `"allowWrite"`), strings.Index(got, `"denyRead"`); idxAllow > idxDeny {
		t.Errorf("denyRead should be appended after existing filesystem keys:\n%s", got)
	}
}

func TestUpdateSettingsFile_RefusesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	// Trailing comma: a hand-edit mistake that must not cost the user their config.
	original := `{"sandbox": {"network": {"allow": []},}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	err := UpdateSettingsFile(path, []string{".env"})
	if err == nil {
		t.Fatal("expected an error instead of an overwrite")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should name the problem, got: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Errorf("file was modified:\n%s", data)
	}
}

func TestUpdateSettingsFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte("  \n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := UpdateSettingsFile(path, []string{".env"}); err != nil {
		t.Fatalf("UpdateSettingsFile error: %v", err)
	}
	s, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if deny := s.GetDenyList(); len(deny) != 1 {
		t.Errorf("GetDenyList() = %v, want [.env]", deny)
	}
}

func TestUpdateSettingsFile_EmptyDenyList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")

	if err := UpdateSettingsFile(path, nil); err != nil {
		t.Fatalf("UpdateSettingsFile error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"denyRead": []`) {
		t.Errorf("an empty deny list should be written as [], got:\n%s", data)
	}
}

func TestUpdateSettingsFile_KeepsFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil { //nolint:gosec // the loose mode is the point: it must survive the rewrite
		t.Fatal(err)
	}
	if err := UpdateSettingsFile(path, []string{".env"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0644 {
		t.Errorf("file mode = %o, want 644", perm)
	}
}

// A non-object value where claudeignore needs an object cannot be kept, but it
// must not take the rest of the file down with it.
func TestUpdateSettingsFile_NonObjectSandbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(`{"sandbox": true, "env": {"A": "1"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSettingsFile(path, []string{".env"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["env"] == nil {
		t.Error("env key was dropped")
	}
	s, _ := ParseSettings(data)
	if deny := s.GetDenyList(); len(deny) != 1 {
		t.Errorf("GetDenyList() = %v, want [.env]", deny)
	}
}

// Raw values are copied, never re-encoded, so characters json.Marshal would
// escape for HTML stay as the user wrote them.
func TestUpdateSettingsFile_NoReEscaping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(`{"cmd": "a && b <c>"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSettingsFile(path, []string{".env"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"a && b <c>"`) {
		t.Errorf("value was re-escaped:\n%s", data)
	}
}

func TestSettings_SetPath(t *testing.T) {
	s, err := ParseSettings([]byte(`{"sandbox":{"network":{"allow":[]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPath(true, "sandbox", "enabled"); err != nil {
		t.Fatal(err)
	}

	out, err := s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	sandbox := m["sandbox"].(map[string]interface{})
	if sandbox["network"] == nil {
		t.Error("sibling key sandbox.network was dropped")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled was not set")
	}
}

func TestParseSettings_Rejects(t *testing.T) {
	cases := map[string]string{
		"not JSON":       `not json`,
		"array":          `[1, 2]`,
		"trailing data":  `{"a": 1} {"b": 2}`,
		"trailing comma": `{"a": 1,}`,
		"unterminated":   `{"a": 1`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSettings([]byte(input)); err == nil {
				t.Errorf("expected an error for %q", input)
			}
		})
	}
}
