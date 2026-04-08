package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const logFileName = "hook.log"
const maxLogSize = 256 * 1024 // 256 KB — rotate when exceeded

// HookLog writes a timestamped entry to .claude/claudeignore/hook.log.
// Silently does nothing if the log directory doesn't exist (project not initialized).
func HookLog(root, hook, message string) {
	logDir := filepath.Join(root, ".claude", "claudeignore")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return
	}

	logPath := filepath.Join(logDir, logFileName)
	rotateIfNeeded(logPath)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(f, "[%s] [%s] %s\n", ts, hook, message)
}

// HookLogError logs an error entry.
func HookLogError(root, hook string, err error) {
	HookLog(root, hook, fmt.Sprintf("ERROR: %v", err))
}

func rotateIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Size() > maxLogSize {
		// Keep .log.old, discard older
		os.Remove(path + ".old")
		os.Rename(path, path+".old")
	}
}
