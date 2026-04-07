package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// setupGitRepo creates a temp git repo with an initial commit.
func setupGitRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

// run executes a command in the given directory, failing the test on error.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec // test helper, args are controlled by test code
	cmd.Dir = dir
	// Allow local file:// transport for submodule tests (required by git >= 2.38.1)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ALLOW_PROTOCOL=file")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed in %s: %v\n%s", name, args, dir, err, out)
	}
}

func TestListSubmodules(t *testing.T) {
	t.Run("no submodules", func(t *testing.T) {
		root := setupGitRepo(t, "parent")
		subs := ListSubmodules(root)
		if len(subs) != 0 {
			t.Errorf("expected no submodules, got %v", subs)
		}
	})

	t.Run("one submodule", func(t *testing.T) {
		sub := setupGitRepo(t, "child")
		root := setupGitRepo(t, "parent")

		run(t, root, "git", "submodule", "add", sub, "libs/child")
		run(t, root, "git", "commit", "-m", "add submodule")

		subs := ListSubmodules(root)
		if len(subs) != 1 || subs[0] != "libs/child" {
			t.Errorf("expected [libs/child], got %v", subs)
		}
	})

	t.Run("multiple submodules", func(t *testing.T) {
		sub1 := setupGitRepo(t, "child1")
		sub2 := setupGitRepo(t, "child2")
		root := setupGitRepo(t, "parent")

		run(t, root, "git", "submodule", "add", sub1, "libs/a")
		run(t, root, "git", "submodule", "add", sub2, "libs/b")
		run(t, root, "git", "commit", "-m", "add submodules")

		subs := ListSubmodules(root)
		sort.Strings(subs)
		if len(subs) != 2 || subs[0] != "libs/a" || subs[1] != "libs/b" {
			t.Errorf("expected [libs/a libs/b], got %v", subs)
		}
	})

	t.Run("not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		subs := ListSubmodules(dir)
		if len(subs) != 0 {
			t.Errorf("expected nil for non-git dir, got %v", subs)
		}
	})
}

func TestGitIgnoredPaths_Submodules(t *testing.T) {
	// Create submodule repo with its own .gitignore and ignored files
	sub := setupGitRepo(t, "child")
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("*.secret\nbuild/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, sub, "git", "add", ".gitignore")
	run(t, sub, "git", "commit", "-m", "add gitignore")

	// Create parent repo
	root := setupGitRepo(t, "parent")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".env\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", ".gitignore")
	run(t, root, "git", "commit", "-m", "add gitignore")

	// Add submodule
	run(t, root, "git", "submodule", "add", sub, "vendor/child")
	run(t, root, "git", "commit", "-m", "add submodule")

	// Create ignored files in parent
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=x"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create ignored files in submodule
	subDir := filepath.Join(root, "vendor", "child")
	if err := os.WriteFile(filepath.Join(subDir, "token.secret"), []byte("tok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(subDir, "build"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "build", "out.bin"), []byte("bin"), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := GitIgnoredPaths(root)
	if err != nil {
		t.Fatalf("GitIgnoredPaths failed: %v", err)
	}

	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	// Parent's .env should be present
	if !pathSet[".env"] {
		t.Errorf("expected .env in paths, got %v", paths)
	}

	// Submodule's ignored files should be present with prefix
	if !pathSet["vendor/child/token.secret"] {
		t.Errorf("expected vendor/child/token.secret in paths, got %v", paths)
	}
	if !pathSet["vendor/child/build"] {
		t.Errorf("expected vendor/child/build in paths, got %v", paths)
	}
}

func TestAllIgnoredPaths_Submodules(t *testing.T) {
	// Create submodule repo
	sub := setupGitRepo(t, "child")
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("*.log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, sub, "git", "add", ".gitignore")
	run(t, sub, "git", "commit", "-m", "add gitignore")

	// Create parent repo
	root := setupGitRepo(t, "parent")

	// Add submodule
	run(t, root, "git", "submodule", "add", sub, "mylib")
	run(t, root, "git", "commit", "-m", "add submodule")

	// Create .claude.ignore that adds extra patterns
	if err := os.WriteFile(filepath.Join(root, ".claude.ignore"), []byte("credentials.json\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create ignored files
	if err := os.WriteFile(filepath.Join(root, "credentials.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(root, "mylib")
	if err := os.WriteFile(filepath.Join(subDir, "app.log"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := AllIgnoredPaths(root)
	if err != nil {
		t.Fatalf("AllIgnoredPaths failed: %v", err)
	}

	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	// .claude.ignore pattern should be picked up
	if !pathSet["credentials.json"] {
		t.Errorf("expected credentials.json in paths, got %v", paths)
	}

	// Submodule's gitignored file should be present
	if !pathSet["mylib/app.log"] {
		t.Errorf("expected mylib/app.log in paths, got %v", paths)
	}
}

func TestAllIgnoredPaths_NoSubmodules(t *testing.T) {
	// Ensure the refactoring didn't break repos without submodules
	root := setupGitRepo(t, "solo")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".env\ndist/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", ".gitignore")
	run(t, root, "git", "commit", "-m", "add gitignore")

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "bundle.js"), []byte("js"), 0600); err != nil {
		t.Fatal(err)
	}

	paths, err := GitIgnoredPaths(root)
	if err != nil {
		t.Fatalf("GitIgnoredPaths failed: %v", err)
	}

	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	if !pathSet[".env"] {
		t.Errorf("expected .env, got %v", paths)
	}
	if !pathSet["dist"] {
		t.Errorf("expected dist, got %v", paths)
	}
}
