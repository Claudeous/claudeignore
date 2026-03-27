package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/claudeous/claudeignore/internal/git"
)

// StateData holds sync state persisted in .claude/.claude.ignore.state.json.
type StateData struct {
	Mode    string   `json:"mode"`              // "gitignore" or "manual"
	Hash    string   `json:"hash"`
	Sync    int64    `json:"sync"`
	NewDeny []string `json:"new_deny,omitempty"` // files added in last sync
}

// StateFilePath returns the path to the state file.
func StateFilePath(root string) string {
	return filepath.Join(root, ".claude", "claudeignore", "state.json")
}

// LoadState reads the state file, returning zero-value on error.
func LoadState(root string) StateData {
	data, err := os.ReadFile(StateFilePath(root))
	if err != nil {
		return StateData{}
	}
	var s StateData
	if err := json.Unmarshal(data, &s); err != nil {
		return StateData{}
	}
	return s
}

// SaveState writes the state to disk.
func SaveState(root string, s StateData) error {
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StateFilePath(root), append(out, '\n'), 0600)
}

// ComputeHash computes a SHA-256 hash of the current config state.
func ComputeHash(root string, mode string) string {
	h := sha256.New()
	if mode == "manual" {
		data, err := os.ReadFile(filepath.Join(root, ".claude.ignore"))
		if err == nil {
			h.Write(data)
		}
	} else {
		for _, name := range []string{".gitignore", ".claude.unignore", ".claude.ignore"} {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err == nil {
				h.Write(data)
			}
		}
		paths, err := git.GitIgnoredPaths(root)
		if err == nil {
			for _, p := range paths {
				h.Write([]byte(p))
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
