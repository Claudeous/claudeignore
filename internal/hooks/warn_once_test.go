package hooks

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestWarnNotGitRepoOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sentinel, err := NoGitWarnedSentinel()
	if err != nil {
		t.Fatalf("NoGitWarnedSentinel: %v", err)
	}

	first := captureStdout(t, WarnNotGitRepoOnce)
	if !strings.Contains(first, "running outside a git repository") {
		t.Errorf("first call should emit warning, got %q", first)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel not created: %v", err)
	}

	second := captureStdout(t, WarnNotGitRepoOnce)
	if second != "" {
		t.Errorf("second call should be silent, got %q", second)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(out)
}
