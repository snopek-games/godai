//go:build !windows

package godot

import (
	"errors"
	"os"
	"syscall"
)

// isProcessRunning reports whether a process with the given PID is currently
// running. On Unix, signalling a process with signal 0 does no actual signalling
// but still performs the usual error checks: a nil error means the process
// exists, and EPERM means it exists but is owned by another user.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
