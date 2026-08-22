//go:build !windows

package harness

import (
	"os"
	"syscall"
)

// Alive reports whether a process the tests didn't launch themselves is still
// running.
func Alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// AskToStop tells a process to shut down. Godot and godai both exit cleanly on
// SIGINT.
func AskToStop(p *os.Process) error {
	return p.Signal(os.Interrupt)
}
