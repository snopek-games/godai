package output

import (
	"os"

	"golang.org/x/sys/windows"
)

// Legacy conhost (e.g. cmd on Windows 10) prints ANSI escapes as garbage
// until virtual terminal processing is switched on, and only Windows 10
// 1511+ can switch it on at all.
func TermSupportsANSI(f *os.File) bool {
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
