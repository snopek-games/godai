//go:build windows

package eval

import "os/exec"

// The harness needs Unix process groups to corral Godot and Claude, so
// cmd/godai-eval refuses to build on Windows; run the eval suite in Docker
// instead. These stubs are never called for real - they exist so the rest of
// this package still compiles here and its tests can run.

func setProcessGroup(*exec.Cmd) {}

func killProcessGroup(int) {}

func killProcess(int) error { return nil }

func processAlive(int) bool { return true }

func strayEditorPids(string) []int { return nil }
