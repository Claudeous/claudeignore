package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const logPrefix = "hook-"
const logExt = ".log"
const maxLogAge = 30 * 24 * time.Hour

// HookLog writes a timestamped entry to .claude/claudeignore/hook-YYYY-MM-DD.log.
// Silently does nothing if the log directory doesn't exist (project not initialized).
// Cleans up log files older than 30 days.
func HookLog(root, hook, message string) {
	logDir := filepath.Join(root, ".claude", "claudeignore")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return
	}

	logPath := filepath.Join(logDir, logPrefix+time.Now().Format("2006-01-02")+logExt)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(f, "[%s] [%s] %s\n", ts, hook, message)

	cleanOldLogs(logDir)
}

// HookLogError logs an error entry.
func HookLogError(root, hook string, err error) {
	HookLog(root, hook, fmt.Sprintf("ERROR: %v", err))
}

func cleanOldLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxLogAge)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, logPrefix) || !strings.HasSuffix(name, logExt) {
			continue
		}
		dateStr := strings.TrimPrefix(strings.TrimSuffix(name, logExt), logPrefix)
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			os.Remove(filepath.Join(dir, name))
		}
	}
}
