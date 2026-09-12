//go:build linux

package core

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestStartVirtualDisplayExitsWhenNothingConnects(t *testing.T) {
	is := is.New(t)
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb is not installed")
	}

	previous := xvfbTerminateDelay
	xvfbTerminateDelay = time.Second
	t.Cleanup(func() { xvfbTerminateDelay = previous })

	display, err := startVirtualDisplay("320x240")
	is.NoErr(err)
	is.True(strings.HasPrefix(display, ":"))

	socket := "/tmp/.X11-unix/X" + display[1:]
	_, err = os.Stat(socket)
	is.NoErr(err) // the display is listening

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(socket); os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Xvfb kept running with no client, so the terminate countdown never started")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestStartVirtualDisplayRejectsMissingXvfb(t *testing.T) {
	is := is.New(t)
	t.Setenv("PATH", t.TempDir())

	_, err := startVirtualDisplay("320x240")
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "Xvfb is not installed"))
}
