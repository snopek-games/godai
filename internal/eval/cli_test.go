package eval

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func cliWorkspace(t *testing.T) *Workspace {
	t.Helper()

	root := t.TempDir()
	godai := filepath.Join(root, "fake-godai")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + filepath.Join(root, "argv") + "\n"
	if err := os.WriteFile(godai, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return &Workspace{
		Root:    root,
		Project: filepath.Join(root, "project"),
		cfg:     Config{GodaiBin: godai, GodotBin: "/opt/godot 4.4/godot"},
	}
}

func TestCLISurfaceResolvesGodaiToTheShim(t *testing.T) {
	is := is.New(t)

	w := cliWorkspace(t)
	s, err := cliSurface(Config{}, w)
	is.NoErr(err)

	which := exec.Command("sh", "-c", "command -v godai")
	which.Env = s.env
	resolved, err := which.Output()
	is.NoErr(err)
	is.Equal(strings.TrimSpace(string(resolved)), filepath.Join(w.Root, "shim", "godai"))

	args := strings.Join(s.args, " ")
	is.True(strings.Contains(args, "Bash,Read,Glob,Grep"))
	is.True(!strings.Contains(args, "Write")) // file tools need --full-tools
	is.True(strings.Contains(args, cliInstructions))
}

func TestCLISurfaceAddsFileToolsWithFullTools(t *testing.T) {
	is := is.New(t)

	full := strings.Join(cliArgs(true), " ")
	is.True(strings.Contains(full, "Bash,Read,Write,Edit,Glob,Grep"))
}

func TestShimSuppliesTheGlobalFlags(t *testing.T) {
	is := is.New(t)

	w := cliWorkspace(t)
	shim, err := w.GodaiShim(w.GodaiArgs()...)
	is.NoErr(err)

	is.NoErr(exec.Command(shim, "editor-tool", "save_scene").Run())

	argv, err := os.ReadFile(filepath.Join(w.Root, "argv"))
	is.NoErr(err)
	is.Equal(strings.Split(strings.TrimSuffix(string(argv), "\n"), "\n"), []string{
		"--root", w.Project,
		"--no-input",
		"--no-auto-install",
		"--godot-path", "/opt/godot 4.4/godot",
		"editor-tool", "save_scene",
	})

	calls, err := w.ShimCalls()
	is.NoErr(err)
	is.Equal(len(calls), 1)
	is.Equal(NormalizeActions(calls)[0].Name, "save_scene")
}

func TestShimAnsweredNeedsAGodaiCallToWorryAbout(t *testing.T) {
	is := is.New(t)

	w := cliWorkspace(t)
	is.NoErr(checkShimAnswered(w, 0))
	is.True(checkShimAnswered(w, 3) != nil) // tool calls happened but none reached the shim

	shim, err := w.GodaiShim(w.GodaiArgs()...)
	is.NoErr(err)
	is.NoErr(exec.Command(shim, "editor", "list").Run())
	is.NoErr(checkShimAnswered(w, 3))
}
