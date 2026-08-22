// Package fakebin writes small executable stand-ins for real binaries, so
// tests can stub out Godot or a client CLI without one installed. Windows
// can't run a shell script, so every stand-in needs a batch version too.
package fakebin

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Write creates an executable at path behaving the way shell and batch
// describe, and returns the path it wrote: on Windows the name gains a
// ".bat" suffix, which exec.LookPath finds from the bare name as well.
func Write(path, shell, batch string) (string, error) {
	body := "#!/bin/sh\n" + shell
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n" + batch
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// Exit writes a stand-in that does nothing but exit with code.
func Exit(path string, code int) (string, error) {
	return Write(path, fmt.Sprintf("exit %d\n", code), fmt.Sprintf("exit /b %d\r\n", code))
}

// Print writes a stand-in that prints line and exits successfully, which is
// how a Godot build reporting its version is stubbed.
func Print(path, line string) (string, error) {
	return Write(path, "echo "+line+"\n", "echo "+line+"\r\n")
}
