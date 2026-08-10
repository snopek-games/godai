//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

// detachProcess puts the child in its own process group so a Ctrl-C/Ctrl-Break
// delivered to our console group doesn't propagate to it.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
