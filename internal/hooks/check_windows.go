//go:build windows

package hooks

// GetClaudeStartTime returns 0 on Windows, disabling restart detection.
//
// On Unix systems this function walks the process tree using `ps` to find
// the parent "claude" process start time. The POSIX `ps` flags used
// (-p PID -o ppid=,lstart=,comm=) are not available on Windows.
//
// Returning 0 means the check hook will never trigger the "restart pending"
// reminder. This is an acceptable degradation because:
//   - The critical guard hook (PreToolUse) works correctly on Windows
//     after the stdin fix (io.ReadAll instead of /dev/stdin).
//   - The sandbox denyRead list is still enforced by Claude Code itself.
//   - Only the convenience "restart to pick up new rules" notification
//     is lost.
//
// A future improvement could use the Windows API (e.g. NtQueryInformationProcess
// or CreateToolhelp32Snapshot) to walk the process tree and retrieve creation
// times, restoring full parity with Unix.
func GetClaudeStartTime() int64 {
	return 0
}
