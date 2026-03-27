package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPathBlocked(t *testing.T) {
	// Create a temporary directory to act as repo root
	root := t.TempDir()

	denyList := []string{
		".env",
		"secrets",
		"config/prod",
		"node_modules",
	}

	tests := []struct {
		name       string
		targetPath string
		blocked    bool
		desc       string
	}{
		{
			name:       "exact match denied file",
			targetPath: filepath.Join(root, ".env"),
			blocked:    true,
			desc:       ".env exactly matches deny list",
		},
		{
			name:       "file inside denied directory",
			targetPath: filepath.Join(root, "secrets", "api_key.txt"),
			blocked:    true,
			desc:       "secrets/api_key.txt is under denied 'secrets' prefix",
		},
		{
			name:       "nested denied directory",
			targetPath: filepath.Join(root, "config", "prod", "db.yml"),
			blocked:    true,
			desc:       "config/prod/db.yml is under denied 'config/prod' prefix",
		},
		{
			name:       "allowed file",
			targetPath: filepath.Join(root, "main.go"),
			blocked:    false,
			desc:       "main.go is not in deny list",
		},
		{
			name:       "allowed nested file",
			targetPath: filepath.Join(root, "config", "dev", "db.yml"),
			blocked:    false,
			desc:       "config/dev is not denied, only config/prod",
		},
		{
			name:       "partial name match not blocked",
			targetPath: filepath.Join(root, ".env_example"),
			blocked:    false,
			desc:       ".env_example should NOT match .env (prefix matching needs /)",
		},
		{
			name:       "node_modules exact",
			targetPath: filepath.Join(root, "node_modules"),
			blocked:    true,
			desc:       "exact match of node_modules",
		},
		{
			name:       "node_modules nested",
			targetPath: filepath.Join(root, "node_modules", "express", "index.js"),
			blocked:    true,
			desc:       "nested file in node_modules",
		},
		{
			name:       "path traversal escaping root",
			targetPath: filepath.Join(root, "..", "outside.txt"),
			blocked:    false,
			desc:       "path escaping repo root should not be blocked",
		},
		{
			name:       "absolute path outside root",
			targetPath: "/tmp/random_file.txt",
			blocked:    false,
			desc:       "absolute path outside repo should not be blocked",
		},
		{
			name:       "empty target path",
			targetPath: "",
			blocked:    false,
			desc:       "empty path should not crash",
		},
		{
			name:       "root itself",
			targetPath: root,
			blocked:    false,
			desc:       "root path itself should not be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason, err := CheckPathBlocked(root, tt.targetPath, denyList)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if blocked != tt.blocked {
				t.Errorf("CheckPathBlocked(%q) blocked=%v, want %v (%s)\nreason: %s",
					tt.targetPath, blocked, tt.blocked, tt.desc, reason)
			}
		})
	}
}

func TestCheckPathBlocked_EmptyDenyList(t *testing.T) {
	root := t.TempDir()

	blocked, _, err := CheckPathBlocked(root, filepath.Join(root, ".env"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("should not block with empty deny list")
	}
}

func TestCheckPathBlocked_SymlinkTraversal(t *testing.T) {
	root := t.TempDir()

	// Create a file outside root
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(secretFile, []byte("secret"), 0644)

	// Create symlink inside root pointing outside
	linkPath := filepath.Join(root, "link_to_secret")
	err := os.Symlink(secretFile, linkPath)
	if err != nil {
		t.Skip("cannot create symlink on this platform")
	}

	denyList := []string{"link_to_secret"}

	// The symlink path itself should match the deny list
	blocked, _, err := CheckPathBlocked(root, linkPath, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("symlink path matching deny entry should be blocked")
	}
}

func TestGuardDenyResponse(t *testing.T) {
	resp := GuardDenyResponse("test reason")
	if len(resp) == 0 {
		t.Fatal("expected non-empty response")
	}

	// Should be valid JSON
	expected := `{"decision":"deny","reason":"test reason"}`
	if string(resp) != expected {
		t.Errorf("got %s, want %s", string(resp), expected)
	}
}
