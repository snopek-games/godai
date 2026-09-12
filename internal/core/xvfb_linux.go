//go:build linux

package core

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// A restarted editor has to reconnect before Xvfb gives up on it, and a
// first-time import on a loaded machine can take a while to reach the point
// of opening a window.
var xvfbTerminateDelay = 60 * time.Second

const xvfbStartTimeout = 15 * time.Second

func startVirtualDisplay(size string) (string, error) {
	xvfb, err := exec.LookPath("Xvfb")
	if err != nil {
		return "", NewUserError("Xvfb is not installed, and an offscreen editor needs it", err, []string{
			"Install it with your package manager, e.g. `apt install xvfb`",
		})
	}

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer readEnd.Close()

	cmd := exec.Command(xvfb,
		"-displayfd", "3",
		"-terminate", strconv.Itoa(int(xvfbTerminateDelay.Seconds())),
		"-screen", "0", size+"x24",
		"-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{writeEnd}
	detachProcess(cmd)

	startErr := cmd.Start()
	writeEnd.Close()
	if startErr != nil {
		return "", NewUserError("unable to start Xvfb for an offscreen editor", startErr, nil)
	}
	go func() { _ = cmd.Wait() }()

	type outcome struct {
		display string
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		line, err := bufio.NewReader(readEnd).ReadString('\n')
		if err != nil {
			done <- outcome{err: fmt.Errorf("Xvfb exited before it was ready: %w", err)}
			return
		}
		number := line[:len(line)-1]
		// Xvfb only starts its -terminate countdown once a client has
		// disconnected, so without this a Godot that never launches would
		// leave it running forever.
		if conn, err := net.Dial("unix", "/tmp/.X11-unix/X"+number); err == nil {
			conn.Close()
		}
		done <- outcome{display: ":" + number}
	}()

	select {
	case result := <-done:
		if result.err != nil {
			_ = cmd.Process.Kill()
			return "", NewUserError("unable to start Xvfb for an offscreen editor", result.err, nil)
		}
		return result.display, nil
	case <-time.After(xvfbStartTimeout):
		_ = cmd.Process.Kill()
		return "", NewUserError("timed out waiting for Xvfb to start for an offscreen editor", nil, nil)
	}
}
