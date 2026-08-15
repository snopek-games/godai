//go:build darwin

package eval

import (
	"os/exec"
	"strconv"
	"strings"
)

func strayEditorPids(scratchDir string) []int {
	out, err := exec.Command("ps", "-axww", "-o", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || !isEditorFor(fields[1:], scratchDir) {
			continue
		}
		if pid, err := strconv.Atoi(fields[0]); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
