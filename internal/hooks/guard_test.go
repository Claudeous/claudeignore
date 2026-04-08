package hooks

import (
	"encoding/json"
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
	if err := os.WriteFile(secretFile, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

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

// --- New tests for Grep guard and related helpers ---

func TestBuildExclusionGlob(t *testing.T) {
	tests := []struct {
		name     string
		denyList []string
		expected string
	}{
		{
			name:     "empty deny list",
			denyList: nil,
			expected: "",
		},
		{
			name:     "single file entry",
			denyList: []string{".env"},
			expected: "!.env",
		},
		{
			name:     "single directory entry",
			denyList: []string{"secrets"},
			expected: "!secrets/**",
		},
		{
			name:     "mixed file and directory entries",
			denyList: []string{".env", "secrets", "node_modules"},
			expected: "!{.env,secrets/**,node_modules/**}",
		},
		{
			name:     "nested directory with dot in file",
			denyList: []string{"config/prod", ".env", "data.json"},
			expected: "!{config/prod/**,.env,data.json}",
		},
		{
			name:     "entry with trailing slash normalized",
			denyList: []string{"secrets/"},
			expected: "!secrets/**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildExclusionGlob("/repo", "/repo", tt.denyList)
			if result != tt.expected {
				t.Errorf("BuildExclusionGlob(%q, %q, %v) = %q, want %q", "/repo", "/repo", tt.denyList, result, tt.expected)
			}
		})
	}
}

func TestIsParentOfDenied(t *testing.T) {
	root := t.TempDir()

	denyList := []string{
		".env",
		"config/prod",
		"secrets",
		"node_modules",
	}

	tests := []struct {
		name       string
		targetPath string
		isParent   bool
	}{
		{
			name:       "config is parent of config/prod",
			targetPath: filepath.Join(root, "config"),
			isParent:   true,
		},
		{
			name:       "secrets is not parent (exact match)",
			targetPath: filepath.Join(root, "secrets"),
			isParent:   false, // "secrets" itself is denied, not a parent
		},
		{
			name:       "src is not parent of any denied entry",
			targetPath: filepath.Join(root, "src"),
			isParent:   false,
		},
		{
			name:       "root is parent of all denied entries",
			targetPath: root,
			isParent:   true, // "." is prefix of everything via Normalize → ""
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isParentOfDenied(root, tt.targetPath, denyList)
			if result != tt.isParent {
				t.Errorf("isParentOfDenied(root, %q, denyList) = %v, want %v",
					tt.targetPath, result, tt.isParent)
			}
		})
	}
}

func TestGuardGrep_NoPathNoGlob(t *testing.T) {
	denyList := []string{".env", "secrets", "node_modules"}
	root := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var) so that root and
	// os.Getwd() use the same physical path prefix.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	// guardGrep uses os.Getwd() as search base when no path is given,
	// so chdir into root so the deny entries resolve correctly.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	toolInput := map[string]interface{}{
		"pattern":     "API_KEY",
		"output_mode": "content",
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be blocked
	if result.Blocked {
		t.Fatal("expected not blocked, got blocked")
	}

	// Should have updatedInput
	if result.UpdatedInput == nil {
		t.Fatal("expected updatedInput to be set")
	}

	// Parse and verify the output
	var output map[string]interface{}
	if err := json.Unmarshal(result.UpdatedInput, &output); err != nil {
		t.Fatalf("invalid JSON in updatedInput: %v", err)
	}

	hookOutput, ok := output["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}

	if hookOutput["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName = %v, want PreToolUse", hookOutput["hookEventName"])
	}
	if hookOutput["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision = %v, want allow", hookOutput["permissionDecision"])
	}

	updatedInput, ok := hookOutput["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatal("missing updatedInput")
	}

	// All original fields must be preserved
	if updatedInput["pattern"] != "API_KEY" {
		t.Errorf("pattern = %v, want API_KEY", updatedInput["pattern"])
	}
	if updatedInput["output_mode"] != "content" {
		t.Errorf("output_mode = %v, want content", updatedInput["output_mode"])
	}

	// Exclusion glob must be set
	glob, ok := updatedInput["glob"].(string)
	if !ok {
		t.Fatal("missing glob in updatedInput")
	}
	if glob != "!{.env,secrets/**,node_modules/**}" {
		t.Errorf("glob = %q, want %q", glob, "!{.env,secrets/**,node_modules/**}")
	}
}

func TestGuardGrep_NoPathGlobIntersects(t *testing.T) {
	root := t.TempDir()
	denyList := []string{".env", "secrets"}

	// Create .env so filepath.Glob can find it
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=x"), 0600); err != nil {
		t.Fatal(err)
	}

	toolInput := map[string]interface{}{
		"pattern": "SECRET",
		"glob":    ".*", // matches .env
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Blocked {
		t.Error("expected blocked when glob intersects deny list")
	}
}

func TestGuardGrep_NoPathGlobNoIntersection(t *testing.T) {
	root := t.TempDir()
	denyList := []string{".env", "secrets"}

	// Create a safe file
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	toolInput := map[string]interface{}{
		"pattern": "func",
		"glob":    "*.go",
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Blocked {
		t.Error("expected not blocked when glob doesn't intersect deny list")
	}

	// Should not have updatedInput (allowed as-is)
	if result.UpdatedInput != nil {
		t.Error("expected no updatedInput for non-intersecting glob")
	}
}

func TestGuardGrep_PathIsParent(t *testing.T) {
	root := t.TempDir()
	denyList := []string{"config/prod", ".env"}

	toolInput := map[string]interface{}{
		"pattern": "password",
		"path":    filepath.Join(root, "config"),
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Blocked {
		t.Error("expected not blocked (should inject updatedInput instead)")
	}

	if result.UpdatedInput == nil {
		t.Fatal("expected updatedInput to be set for parent path")
	}

	// Verify the updatedInput preserves original fields
	var output map[string]interface{}
	if err := json.Unmarshal(result.UpdatedInput, &output); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hookOutput := output["hookSpecificOutput"].(map[string]interface{})
	updatedInput := hookOutput["updatedInput"].(map[string]interface{})

	if updatedInput["pattern"] != "password" {
		t.Errorf("pattern = %v, want password", updatedInput["pattern"])
	}
	if updatedInput["path"] != filepath.Join(root, "config") {
		t.Errorf("path = %v, want %s", updatedInput["path"], filepath.Join(root, "config"))
	}
	if _, ok := updatedInput["glob"].(string); !ok {
		t.Error("expected glob to be set in updatedInput")
	}
}

func TestGuardGrep_PathDenied(t *testing.T) {
	root := t.TempDir()
	denyList := []string{"secrets", ".env"}

	toolInput := map[string]interface{}{
		"pattern": "key",
		"path":    filepath.Join(root, "secrets"),
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Blocked {
		t.Error("expected blocked when path is directly denied")
	}
}

func TestGuardGrep_PathUnrelated(t *testing.T) {
	root := t.TempDir()
	denyList := []string{".env", "secrets"}

	toolInput := map[string]interface{}{
		"pattern": "func",
		"path":    filepath.Join(root, "src"),
	}

	result, err := guardGrep(root, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Blocked {
		t.Error("expected not blocked for unrelated path")
	}
	if result.UpdatedInput != nil {
		t.Error("expected no updatedInput for unrelated path")
	}
}

func TestGlobPatternMatchesDenyList(t *testing.T) {
	denyList := []string{".env", "secrets", "config/prod"}

	tests := []struct {
		name    string
		glob    string
		matches bool
	}{
		{
			name:    "glob matches denied file",
			glob:    ".env",
			matches: true,
		},
		{
			name:    "glob wildcard matches denied file",
			glob:    ".*",
			matches: true, // matches .env
		},
		{
			name:    "glob targets denied directory",
			glob:    "secrets/*",
			matches: true,
		},
		{
			name:    "glob doesn't match any denied",
			glob:    "*.go",
			matches: false,
		},
		{
			name:    "glob matches denied nested path",
			glob:    "config/prod",
			matches: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := globPatternMatchesDenyList(tt.glob, denyList)
			if result != tt.matches {
				t.Errorf("globPatternMatchesDenyList(%q) = %v, want %v", tt.glob, result, tt.matches)
			}
		})
	}
}

func TestGlobIntersectsDenyList_WithFiles(t *testing.T) {
	root := t.TempDir()
	denyList := []string{".env", "secrets"}

	// Create the denied file so glob resolution finds it
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// Create a safe file
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// Glob that matches .env
	if !globIntersectsDenyList(root, ".*", denyList) {
		t.Error("expected .* to intersect deny list (matches .env)")
	}

	// Glob that only matches safe files
	if globIntersectsDenyList(root, "*.md", denyList) {
		t.Error("expected *.md not to intersect deny list")
	}
}

func TestBuildUpdatedInputJSON(t *testing.T) {
	denyList := []string{".env", "secrets"}
	toolInput := map[string]interface{}{
		"pattern":     "password",
		"output_mode": "content",
		"-n":          true,
	}

	result, err := buildUpdatedInputJSON("/repo", "/repo", toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hookOutput, ok := output["hookSpecificOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}

	if hookOutput["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName = %v, want PreToolUse", hookOutput["hookEventName"])
	}
	if hookOutput["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision = %v, want allow", hookOutput["permissionDecision"])
	}

	updatedInput, ok := hookOutput["updatedInput"].(map[string]interface{})
	if !ok {
		t.Fatal("missing updatedInput")
	}

	// Verify all original fields preserved
	if updatedInput["pattern"] != "password" {
		t.Errorf("pattern not preserved")
	}
	if updatedInput["output_mode"] != "content" {
		t.Errorf("output_mode not preserved")
	}
	if updatedInput["-n"] != true {
		t.Errorf("-n not preserved")
	}

	// Verify glob is set
	glob, ok := updatedInput["glob"].(string)
	if !ok || glob == "" {
		t.Fatal("glob not set in updatedInput")
	}
	if glob != "!{.env,secrets/**}" {
		t.Errorf("glob = %q, want %q", glob, "!{.env,secrets/**}")
	}
}

// TestGuard_ReadsStdin verifies that the Guard function reads hook input from
// os.Stdin (via io.ReadAll) rather than /dev/stdin. This is the critical fix
// for Windows compatibility — /dev/stdin does not exist on Windows, causing
// the guard to silently fail-open with no file protection.
func TestGuard_ReadsStdin(t *testing.T) {
	root := t.TempDir()

	// Create settings.local.json with a deny list
	settingsDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(settingsDir, 0750); err != nil {
		t.Fatal(err)
	}
	settingsJSON := `{"sandbox":{"filesystem":{"denyRead":[".env","secrets"]}}}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.local.json"), []byte(settingsJSON), 0600); err != nil {
		t.Fatal(err)
	}

	// Prepare hook input JSON (simulates what Claude Code pipes into stdin)
	hookInput := `{"tool_name":"Read","tool_input":{"file_path":"` + filepath.Join(root, ".env") + `"}}`

	// Replace os.Stdin with a pipe containing our hook input
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	// Write the hook input and close the write end so ReadAll returns
	if _, err := w.WriteString(hookInput); err != nil {
		t.Fatal(err)
	}
	w.Close()

	result, err := Guard(root)
	if err != nil {
		t.Fatalf("Guard returned error: %v", err)
	}

	if !result.Blocked {
		t.Error("expected Guard to block .env access, but it allowed it — stdin reading may be broken")
	}
	if result.Reason == "" {
		t.Error("expected a deny reason, got empty string")
	}
}

// TestGuard_ReadsStdin_AllowedFile verifies that allowed files pass through
// when stdin is read correctly.
func TestGuard_ReadsStdin_AllowedFile(t *testing.T) {
	root := t.TempDir()

	// Create settings.local.json with a deny list
	settingsDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(settingsDir, 0750); err != nil {
		t.Fatal(err)
	}
	settingsJSON := `{"sandbox":{"filesystem":{"denyRead":[".env","secrets"]}}}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.local.json"), []byte(settingsJSON), 0600); err != nil {
		t.Fatal(err)
	}

	// Request an allowed file
	hookInput := `{"tool_name":"Read","tool_input":{"file_path":"` + filepath.Join(root, "main.go") + `"}}`

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	if _, err := w.WriteString(hookInput); err != nil {
		t.Fatal(err)
	}
	w.Close()

	result, err := Guard(root)
	if err != nil {
		t.Fatalf("Guard returned error: %v", err)
	}

	if result.Blocked {
		t.Error("expected Guard to allow main.go access, but it blocked it")
	}
}

func TestBuildUpdatedInputJSON_EmptyDenyList(t *testing.T) {
	toolInput := map[string]interface{}{
		"pattern": "test",
	}

	_, err := buildUpdatedInputJSON("/repo", "/repo", toolInput, nil)
	if err == nil {
		t.Error("expected error for empty deny list")
	}
}

func TestBuildExclusionGlob_Subdirectory(t *testing.T) {
	root := "/repo"

	tests := []struct {
		name       string
		searchBase string
		denyList   []string
		expected   string
	}{
		{
			name:       "search from subdirectory containing denied file",
			searchBase: "/repo/config",
			denyList:   []string{"config/secrets.yaml"},
			expected:   "!secrets.yaml",
		},
		{
			name:       "search from subdirectory with nested deny",
			searchBase: "/repo/config",
			denyList:   []string{"config/prod"},
			expected:   "!prod/**",
		},
		{
			name:       "search from repo root (no change)",
			searchBase: "/repo",
			denyList:   []string{".env", "secrets"},
			expected:   "!{.env,secrets/**}",
		},
		{
			name:       "deny entry outside search base is skipped",
			searchBase: "/repo/src",
			denyList:   []string{".env", "config/secrets.yaml"},
			expected:   "",
		},
		{
			name:       "mixed: some inside, some outside search base",
			searchBase: "/repo/config",
			denyList:   []string{".env", "config/secrets.yaml", "config/prod"},
			expected:   "!{secrets.yaml,prod/**}",
		},
		{
			name:       "search from root with single deny",
			searchBase: "/repo",
			denyList:   []string{".env"},
			expected:   "!.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildExclusionGlob(root, tt.searchBase, tt.denyList)
			if result != tt.expected {
				t.Errorf("BuildExclusionGlob(%q, %q, %v) = %q, want %q",
					root, tt.searchBase, tt.denyList, result, tt.expected)
			}
		})
	}
}

// --- Integration tests for guardGrep from subdirectory ---

func TestGuardGrep_SubdirectoryNoPath(t *testing.T) {
	// Simulate: repo at root, Claude launched from root/config (cwd)
	root := t.TempDir()

	// Create denied file
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "secrets.yaml"), []byte("API_KEY=secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "app.yaml"), []byte("name: myapp"), 0600); err != nil {
		t.Fatal(err)
	}

	denyList := []string{"config/secrets.yaml"}

	// Change cwd to the subdirectory (simulating Claude launched from there)
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalCwd) }()

	// Resolve symlinks for macOS (/var -> /private/var)
	resolvedConfig, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(resolvedConfig); err != nil {
		t.Fatal(err)
	}

	// Grep with no path, no glob → should inject exclusion relative to cwd
	toolInput := map[string]interface{}{
		"pattern": "API_KEY",
	}

	result, err := guardGrep(resolvedRoot, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Blocked {
		t.Fatal("expected not blocked, got blocked")
	}
	if result.UpdatedInput == nil {
		t.Fatal("expected updatedInput to be set")
	}

	// Parse the injected glob
	var output map[string]interface{}
	if err := json.Unmarshal(result.UpdatedInput, &output); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hookOutput := output["hookSpecificOutput"].(map[string]interface{})
	updatedInput := hookOutput["updatedInput"].(map[string]interface{})
	glob := updatedInput["glob"].(string)

	// The glob must be relative to the cwd (config/), not the repo root
	if glob != "!secrets.yaml" {
		t.Errorf("glob = %q, want %q (must be relative to cwd, not repo root)", glob, "!secrets.yaml")
	}
}

func TestGuardGrep_SubdirectoryWithPath(t *testing.T) {
	root := t.TempDir()

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "prod"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "prod", "db.yml"), []byte("password: secret"), 0600); err != nil {
		t.Fatal(err)
	}

	// Resolve symlinks for macOS
	resolvedConfig, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	denyList := []string{"config/prod"}

	toolInput := map[string]interface{}{
		"pattern": "password",
		"path":    resolvedConfig, // explicit path = the search base
	}

	result, err := guardGrep(resolvedRoot, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Blocked {
		t.Fatal("expected not blocked (should inject updatedInput)")
	}
	if result.UpdatedInput == nil {
		t.Fatal("expected updatedInput to be set")
	}

	var output map[string]interface{}
	if err := json.Unmarshal(result.UpdatedInput, &output); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hookOutput := output["hookSpecificOutput"].(map[string]interface{})
	updatedInput := hookOutput["updatedInput"].(map[string]interface{})
	glob := updatedInput["glob"].(string)

	// "config/prod" relative to "config/" = "prod"
	if glob != "!prod/**" {
		t.Errorf("glob = %q, want %q (must be relative to search path, not repo root)", glob, "!prod/**")
	}
}

func TestGuardGrep_DenyOutsideSearchBase(t *testing.T) {
	root := t.TempDir()

	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0750); err != nil {
		t.Fatal(err)
	}

	denyList := []string{".env", "config/secrets.yaml"}

	// Change cwd to src/ — neither .env nor config/secrets.yaml is under src/
	originalCwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalCwd) }()

	resolvedSrc, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(resolvedSrc); err != nil {
		t.Fatal(err)
	}

	toolInput := map[string]interface{}{
		"pattern": "import",
	}

	result, err := guardGrep(resolvedRoot, toolInput, denyList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// buildUpdatedInputJSON returns error for empty glob → guardGrep fails open
	if result.Blocked {
		t.Error("expected not blocked when deny entries are outside search base")
	}
	if result.UpdatedInput != nil {
		t.Error("expected no updatedInput when deny entries are outside search base")
	}
}
