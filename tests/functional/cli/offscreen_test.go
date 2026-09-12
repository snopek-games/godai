package cli

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func TestProjectOpenOffscreen(t *testing.T) {
	is := is.New(t)
	if runtime.GOOS != "linux" {
		t.Skip("offscreen editors are only supported on Linux")
	}
	if _, err := exec.LookPath("Xvfb"); err != nil {
		t.Skip("Xvfb is not installed")
	}

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	dir := newProjectDir(t, "offscreen-open")

	env, err := isolation.Env(harness.IsolationDir(projectPath))
	is.NoErr(err)
	env = append(env, "GODAI_EDITOR_LOG=1")

	t.Cleanup(func() {
		_, _ = godaiWithEnv(env, "editor", "close", dir, "--skip-save")
	})

	out, err := godaiWithEnv(env, "project", "open", dir,
		"--godot-path", godotBin, "--offscreen", "--offscreen-size", "800x600", "--auto-approve")
	if err != nil {
		t.Fatalf("project open: %v\n%s", err, out)
	}

	// The editor can still be letting go of the opening process's connection
	// when the listing process arrives, so the first listing may miss it.
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err = godaiWithEnv(env, "--json", "editor", "list")
		if err != nil {
			t.Fatalf("editor list: %v\n%s", err, out)
		}
		var listed struct {
			Projects []struct {
				ProjectPath string `json:"project_path"`
				Headless    bool   `json:"headless"`
				Offscreen   bool   `json:"offscreen"`
			} `json:"projects"`
		}
		is.NoErr(json.Unmarshal([]byte(out), &listed))

		for _, p := range listed.Projects {
			if p.ProjectPath != dir {
				continue
			}
			is.True(p.Offscreen) // the addon reported the virtual display godai gave it
			is.True(!p.Headless)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the offscreen editor %s is not listed:\n%s", dir, out)
		}
		time.Sleep(time.Second)
	}
}
