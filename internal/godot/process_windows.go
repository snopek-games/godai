//go:build windows

package godot

import "syscall"

// stillActive is the exit code (STILL_ACTIVE / STATUS_PENDING) reported by
// GetExitCodeProcess for a process that hasn't terminated yet.
const stillActive = 259

// isProcessRunning reports whether a process with the given PID is currently
// running. On Windows we open the process and read its exit code: a process
// that is still alive reports STILL_ACTIVE.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		// The process doesn't exist (or we can't open it at all).
		return false
	}
	defer syscall.CloseHandle(handle)

	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
