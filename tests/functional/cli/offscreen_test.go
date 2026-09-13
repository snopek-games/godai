package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func TestProjectOpenOffscreen(t *testing.T) {
	is := is.New(t)
	dir, env := openOffscreenEditor(t, "offscreen-open")

	// The editor can still be letting go of the opening process's connection
	// when the listing process arrives, so the first listing may miss it.
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := godaiWithEnv(env, "--json", "editor", "list")
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

func TestTakeScreenshotOffscreen(t *testing.T) {
	is := is.New(t)
	dir, env := openOffscreenEditor(t, "offscreen-screenshot")

	named := filepath.Join(t.TempDir(), "editor.png")
	out, err := godaiWithEnv(env, "editor-tool", "take_screenshot", "--project-path", dir, "--output", named)
	if err != nil {
		t.Fatalf("take_screenshot --output: %v\n%s", err, out)
	}
	is.Equal(out, "") // naming the file leaves nothing to report
	checkPNG(t, named)

	out, err = godaiWithEnv(env, "editor-tool", "take_screenshot", "--project-path", dir)
	if err != nil {
		t.Fatalf("take_screenshot: %v\n%s", err, out)
	}
	printed := strings.TrimSpace(out)
	t.Cleanup(func() { os.Remove(printed) })
	is.True(strings.HasSuffix(printed, ".png")) // the printed path is the temp file
	checkPNG(t, printed)

	out, err = godaiWithEnv(env, "--json", "editor-tool", "take_screenshot", "--project-path", dir)
	if err != nil {
		t.Fatalf("take_screenshot --json: %v\n%s", err, out)
	}
	var structured struct {
		Success bool `json:"success"`
		Output  struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"output"`
	}
	is.NoErr(json.Unmarshal([]byte(out), &structured))
	is.True(structured.Success)
	is.Equal(structured.Output.MimeType, "image/png")
	data, err := base64.StdEncoding.DecodeString(structured.Output.Data)
	is.NoErr(err)
	_, err = png.Decode(bytes.NewReader(data))
	is.NoErr(err) // --json without --output carries the image in the output field

	out, err = godaiWithEnv(env, "--json", "editor-tool", "take_screenshot", "--project-path", dir, "--output", filepath.Join(t.TempDir(), "editor.png"))
	if err != nil {
		t.Fatalf("take_screenshot --json --output: %v\n%s", err, out)
	}
	is.True(!strings.Contains(out, `"output"`)) // the file holds the image instead
}

func checkPNG(t *testing.T, path string) {
	t.Helper()
	is := is.New(t)

	data, err := os.ReadFile(path)
	is.NoErr(err)
	decoded, err := png.Decode(bytes.NewReader(data))
	is.NoErr(err)
	bounds := decoded.Bounds()
	is.True(bounds.Dx() > 0 && bounds.Dy() > 0)
	is.True(bounds.Dx() <= 800 && bounds.Dy() <= 600) // no bigger than the virtual display
}

// Opens a second, offscreen editor on a fresh project and closes it when the
// test ends. Skips where offscreen editors can't run.
func openOffscreenEditor(t *testing.T, name string) (dir string, env []string) {
	t.Helper()
	is := is.New(t)

	if err := harness.CheckOffscreen(); err != nil {
		t.Skip(err.Error())
	}

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	dir = newProjectDir(t, name, harness.ProjectOptions{RenderingMethod: harness.OffscreenRenderingMethod})

	env, err = isolation.Env(harness.IsolationDir(projectPath))
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
	return dir, env
}
