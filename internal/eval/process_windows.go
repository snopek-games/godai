//go:build windows

package eval

import "os/exec"

// The harness needs Unix process groups to corral Godot and Claude; on
// Windows, run the eval suite in Docker. The stubs keep this the only error.
var _ = theEvalSuiteDoesNotBuildOnWindows_useDockerInstead

func setProcessGroup(*exec.Cmd) {}

func killProcessGroup(int) {}

func killProcess(int) error { return nil }

func processAlive(int) bool { return true }

func strayEditorPids(string) []int { return nil }
