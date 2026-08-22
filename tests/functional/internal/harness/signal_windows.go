//go:build windows

package harness

import (
	"os"
	"syscall"
)

// Alive reports whether a process the tests didn't launch themselves is still
// running. Windows has no signal 0 to probe with, so its handle is waited on
// for no time at all instead: a process that hasn't exited never signals.
func Alive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION|syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	event, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && event == uint32(syscall.WAIT_TIMEOUT)
}

// AskToStop terminates a process. Windows has no signal to ask a windowless
// process to quit; the server, which needs to exit gracefully, is stopped by
// closing its stdin instead.
func AskToStop(p *os.Process) error {
	return p.Kill()
}
