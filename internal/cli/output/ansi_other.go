//go:build !windows

package output

import "os"

func TermSupportsANSI(f *os.File) bool {
	return true
}
