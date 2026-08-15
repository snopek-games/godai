//go:build linux

package eval

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func strayEditorPids(scratchDir string) []int {
	entries, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	var pids []int
	for _, entry := range entries {
		cmdline, err := os.ReadFile(entry)
		if err != nil || !isEditorFor(strings.Split(string(cmdline), "\x00"), scratchDir) {
			continue
		}
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(entry)))
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
