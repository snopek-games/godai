//go:build windows

package main

// The harness needs Unix process groups to corral Godot and Claude; on
// Windows, run the eval suite in Docker. Failing here rather than in
// internal/eval keeps that package's tests runnable on Windows, while still
// making a Windows build of this binary the only error.
var _ = theEvalSuiteDoesNotBuildOnWindows_useDockerInstead
