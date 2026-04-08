//go:build !windows

package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GetClaudeStartTime walks up the process tree to find the "claude" process
// start time. It uses the POSIX `ps` command, which is available on Linux,
// macOS, and other Unix-like systems.
func GetClaudeStartTime() int64 {
	pid := os.Getppid()
	for i := 0; i < 10; i++ {
		if pid <= 1 {
			break
		}
		out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "ppid=,lstart=,comm=").Output() //nolint:gosec // pid is from os.Getppid
		if err != nil {
			break
		}
		fields := strings.TrimSpace(string(out))
		parts := strings.Fields(fields)
		if len(parts) < 7 {
			break
		}
		ppid, _ := strconv.Atoi(parts[0])
		comm := parts[len(parts)-1]
		dateStr := strings.Join(parts[1:len(parts)-1], " ")

		if strings.Contains(comm, "claude") {
			loc := time.Now().Location()
			t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", dateStr, loc)
			if err != nil {
				t, err = time.ParseInLocation("Mon Jan  2 15:04:05 2006", dateStr, loc)
			}
			if err == nil {
				return t.Unix()
			}
			return 0
		}
		pid = ppid
	}
	return 0
}
