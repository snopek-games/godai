//go:build !windows

package server

import (
	"os/exec"
	"syscall"
)

// detachProcess puts the child in its own process group so signals sent to the
// server's group (e.g. a Ctrl-C in the launching terminal) don't tear it down.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
