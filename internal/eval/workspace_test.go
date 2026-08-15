package eval

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestCleanupKillsAnEditorItCouldNotClose(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	w := &Workspace{Root: root, Project: filepath.Join(root, "project"), cfg: Config{KeepWork: true}}

	// The two arguments are the only things killStrayEditors matches on.
	stray := exec.Command("/bin/sh", "-c", "sleep 60; :", filepath.Base(root), "--editor")
	is.NoErr(stray.Start())

	done := make(chan error, 1)
	go func() { done <- stray.Wait() }()

	w.Cleanup(context.Background())

	select {
	case err := <-done:
		is.True(err != nil) // killed rather than exited on its own
	case <-time.After(10 * time.Second):
		_ = stray.Process.Kill()
		t.Fatal("the stray editor was left running")
	}
}

func TestStrayEditorMatchIsNotFooledByGodaisOwnFlags(t *testing.T) {
	is := is.New(t)

	scratch := "godai-eval-add-player-sprite-r1-1234"
	project := "/tmp/" + scratch + "/project"

	is.True(isEditorFor([]string{"/usr/local/bin/godot", "--editor", "--path", project,
		"--display-driver", "headless"}, scratch))

	is.True(isEditorFor([]string{"/usr/local/bin/godot", "--path", project, "--remote-debug",
		"tcp://127.0.0.1:6007", "--editor-pid", "12345", "res://main.tscn"}, scratch))

	for _, notAnEditor := range [][]string{
		{"godai", "--root", project, "--editor-timeout", "30", "editor", "close", project},
		{"godai", "--root", project, "--no-input", "editor-tool", "save_scene"},
		{"godai", "--root", project, "mcp", "--no-update-check"},
		{"/usr/local/bin/godot", "--editor", "--path", "/tmp/someone-elses-project"},
	} {
		is.True(!isEditorFor(notAnEditor, scratch))
	}
}
