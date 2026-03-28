package hooks

import (
	"os"
	"testing"

	"github.com/claudeous/claudeignore/internal/config"
)

func TestCheck_NoStateFile_ReturnsNil(t *testing.T) {
	// Create a temp dir as repo root with no state.json — simulates a project
	// that was never initialized with claudeignore.
	root := t.TempDir()

	result, err := Check(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for uninitialized project, got %+v", result)
	}
}

func TestCheck_WithStateFile_DoesNotReturnNil(t *testing.T) {
	// Create a temp dir with a state.json — simulates an initialized project.
	// With an empty hash, Check should detect NeedsSync and return a non-nil result.
	root := t.TempDir()

	// Create the state directory and file
	err := os.MkdirAll(config.StateFilePath(root)[:len(config.StateFilePath(root))-len("state.json")], 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = config.SaveState(root, config.StateData{Mode: "gitignore"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := Check(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// State file exists with empty hash, so NeedsSync should be true
	if result == nil {
		t.Fatal("expected non-nil result for initialized project with empty hash")
	}
	if !result.NeedsSync {
		t.Error("expected NeedsSync=true for state with empty hash")
	}
}
