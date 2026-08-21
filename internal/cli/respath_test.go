package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/cli/schemaflag"
	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func testProject(t *testing.T) string {
	t.Helper()

	project := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		project = resolved
	}
	return project
}

func TestResPathRewritesLocalPaths(t *testing.T) {
	is := is.New(t)

	project := testProject(t)
	is.NoErr(os.MkdirAll(filepath.Join(project, "scenes"), 0o755))

	t.Chdir(project)

	for input, want := range map[string]string{
		"demo.tscn":           "res://demo.tscn",
		"./demo.tscn":         "res://demo.tscn",
		"scenes/demo.tscn":    "res://scenes/demo.tscn",
		"scenes/../spin.gd":   "res://spin.gd",
		"new/not/yet/here.gd": "res://new/not/yet/here.gd",
		filepath.Join(project, "scenes", "demo.tscn"): "res://scenes/demo.tscn",
		"res://demo.tscn":  "res://demo.tscn",
		"user://saves.cfg": "user://saves.cfg",
		"uid://c4f2xyz":    "uid://c4f2xyz",
		"":                 "",
	} {
		got, err := resPath(input, project, "file-path")
		is.NoErr(err)
		is.Equal(got, want)
	}
}

func TestResPathIsRelativeToTheWorkingDirectory(t *testing.T) {
	is := is.New(t)

	project := testProject(t)
	is.NoErr(os.MkdirAll(filepath.Join(project, "scenes"), 0o755))

	t.Chdir(filepath.Join(project, "scenes"))

	got, err := resPath("demo.tscn", project, "file-path")
	is.NoErr(err)
	is.Equal(got, "res://scenes/demo.tscn")

	got, err = resPath("../spin.gd", project, "file-path")
	is.NoErr(err)
	is.Equal(got, "res://spin.gd")
}

func TestResPathRejectsPathsOutsideTheProject(t *testing.T) {
	is := is.New(t)

	project := testProject(t)
	elsewhere := testProject(t)

	t.Chdir(elsewhere)

	for _, input := range []string{
		"demo.tscn",
		filepath.Join(elsewhere, "demo.tscn"),
		filepath.Join(project, "..", "demo.tscn"),
	} {
		_, err := resPath(input, project, "file-path")
		is.True(err != nil)
		is.True(strings.Contains(err.Error(), "outside the project"))
		is.True(strings.Contains(err.Error(), "--file-path")) // the error names the flag
		is.Equal(ExitCodeFor(err), ExitUsage)
	}
}

func TestNormalizeResPathArgs(t *testing.T) {
	is := is.New(t)

	project := testProject(t)
	t.Chdir(project)

	specs := []schemaflag.Spec{
		{Property: "file_path", Flag: "file-path", Kind: schemaflag.KindString},
		{Property: "file_paths", Flag: "file-paths", Kind: schemaflag.KindStringList},
	}

	args := core.Args{}
	is.NoErr(args.Set("file_path", "spin.gd"))
	is.NoErr(args.Set("file_paths", []string{"a.png", "res://b.png"}))
	is.NoErr(args.Set("content", "extends Node"))

	is.NoErr(normalizeResPathArgs(args, specs, project))

	is.Equal(string(args["file_path"]), `"res://spin.gd"`)
	is.Equal(string(args["file_paths"]), `["res://a.png","res://b.png"]`)
	is.Equal(string(args["content"]), `"extends Node"`) // non-path args are untouched
}

func TestNormalizeResPathArgsLeavesWrongShapesForTheEditor(t *testing.T) {
	is := is.New(t)

	specs := []schemaflag.Spec{
		{Property: "file_path", Flag: "file-path", Kind: schemaflag.KindString},
	}

	args := core.Args{}
	args.SetRaw("file_path", json.RawMessage(`42`))

	is.NoErr(normalizeResPathArgs(args, specs, testProject(t)))
	is.Equal(string(args["file_path"]), `42`)
}
