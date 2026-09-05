package harness

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockMachine coordinates the functional test suites when `go test` runs their
// packages in parallel. Suites that tolerate sharing the machine lock it
// shared and still run concurrently with each other; wall-clock-sensitive
// suites lock it exclusive to get the machine to themselves. Blocks until the
// lock is acquired and returns a release function.
func LockMachine(exclusive bool) (func(), error) {
	path := filepath.Join(os.TempDir(), "godai-functional-tests.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, fmt.Errorf("opening machine lock: %w", err)
	}
	if err := lockFile(f, exclusive); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring machine lock %s: %w", path, err)
	}
	return func() { f.Close() }, nil
}
