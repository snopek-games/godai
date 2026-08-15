package eval

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

// A --content argument holds anything a line or a shell word could be split on.
func TestShimRecordsCallsWithAwkwardArguments(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	godai := filepath.Join(root, "fake-godai")
	is.NoErr(os.WriteFile(godai, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	w := &Workspace{Root: root, cfg: Config{GodaiBin: godai}}
	shim, err := w.GodaiShim()
	is.NoErr(err)

	for _, argv := range [][]string{
		{"--root", root, "editor-tool", "create_script", "--content", "extends Node\n\nfunc _ready():\n\tpass\n", "--label", ""},
		{"--root", root, "editor-tool", "save_scene"},
	} {
		is.NoErr(exec.Command(shim, argv...).Run())
	}

	calls, err := w.ShimCalls()
	is.NoErr(err)
	is.Equal(len(calls), 2)

	actions := NormalizeActions(calls)
	is.Equal(actions[0].Kind, ActionKindGodai)
	is.Equal(actions[0].Name, "create_script")
	is.Equal(actions[1].Name, "save_scene")
}

func TestShimEchoesCommandsOnlyWhenAsked(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	godai := filepath.Join(root, "fake-godai")
	is.NoErr(os.WriteFile(godai, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	w := &Workspace{Root: root, cfg: Config{GodaiBin: godai}}
	shim, err := w.GodaiShim()
	is.NoErr(err)

	quiet := exec.Command(shim, "editor-tool", "save_scene")
	out, err := quiet.CombinedOutput()
	is.NoErr(err)
	is.Equal(string(out), "")

	echoed := exec.Command(shim, "editor-tool", "save_scene")
	echoed.Env = append(os.Environ(), echoCommandsEnv+"=1")
	out, err = echoed.CombinedOutput()
	is.NoErr(err)
	is.Equal(string(out), "$ godai editor-tool save_scene\n")
}
