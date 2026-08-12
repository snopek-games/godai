package mcp

import (
	"path/filepath"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

// extraEngineName is removed by these tests, so that the engine the rest of
// the suite runs on is left alone. There's no tool that links one, so it goes
// into engines.json the same way the test engine does.
const extraEngineName = "extra-build"

func linkExtraEngine(t *testing.T) {
	t.Helper()

	err := writeLinkedEngines(map[string]any{
		testEngineName:  map[string]string{"path": godotWrapperPath},
		extraEngineName: map[string]string{"path": godotWrapperPath},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func findEngine(engines []any, version string) map[string]any {
	for _, e := range engines {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if m["version"] == version {
			return m
		}
	}
	return nil
}

func listEngines(t *testing.T) []any {
	t.Helper()

	structured := callToolOK(t, "list_installed_godot_versions", nil)
	versions, ok := structured["versions"].([]any)
	if !ok {
		t.Fatalf("list_installed_godot_versions has no versions array: %v", structured)
	}
	return versions
}

func TestListInstalledGodotVersions(t *testing.T) {
	is := is.New(t)

	engine := findEngine(listEngines(t), testEngineName)
	is.True(engine != nil) // the linked test engine is listed
	is.Equal(engine["linked"], true)
	is.Equal(engine["path"], godotWrapperPath)
}

func TestRemoveGodotVersion(t *testing.T) {
	is := is.New(t)

	linkExtraEngine(t)
	is.True(findEngine(listEngines(t), extraEngineName) != nil)

	removed := callToolOK(t, "remove_godot_version", map[string]any{
		"godot_version": extraEngineName,
	})
	is.Equal(removed["version"], extraEngineName)
	is.Equal(removed["linked"], true)
	is.Equal(removed["default_cleared"], false) // it was never the default

	is.True(findEngine(listEngines(t), extraEngineName) == nil)
	is.True(findEngine(listEngines(t), testEngineName) != nil) // the one the suite runs on

	// A linked name that's gone isn't a version either, so that's what it's
	// reported as.
	callToolErr(t, "remove_godot_version", map[string]any{
		"godot_version": extraEngineName,
	}, "isn't a Godot version")

	callToolErr(t, "remove_godot_version", map[string]any{
		"godot_version": "4.5-stable",
	}, "isn't installed")
}

// An editor keeps the Godot it was started with, so asking for another one is
// refused rather than quietly ignored.
func TestOpenGodotProjectRejectsAVersionTheOpenEditorIsnt(t *testing.T) {
	t.Cleanup(func() {
		if err := writeLinkedEngines(map[string]any{
			testEngineName: map[string]string{"path": godotWrapperPath},
		}); err != nil {
			t.Fatal(err)
		}
	})

	// Godot 3 can't be what the editor is running, and recording a version
	// saves standing up a second build to compare against.
	err := writeLinkedEngines(map[string]any{
		testEngineName: map[string]string{"path": godotWrapperPath},
		"old-build":    map[string]string{"path": godotWrapperPath, "version": "3.5-stable"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ensureProjectOpen(t)

	callToolErr(t, "open_godot_project", map[string]any{
		"project_path":  projectPath,
		"godot_version": "old-build",
	}, "not old-build")
}

// A project that was never opened keeps this from depending on whether an
// editor is already running, which is what would otherwise answer first.
func TestOpenGodotProjectRejectsAnUnknownGodotVersion(t *testing.T) {
	is := is.New(t)

	dir := filepath.Join(filepath.Dir(projectPath), "unopened")
	is.NoErr(harness.CreateTestProject(dir, harness.ProjectOptions{Name: "Unopened"}))

	callToolErr(t, "open_godot_project", map[string]any{
		"project_path":  dir,
		"godot_version": "not-a-build",
	}, "isn't a Godot version")
}

func TestPinAndUnpinProjectGodotVersion(t *testing.T) {
	is := is.New(t)

	pinned := callToolOK(t, "pin_project_to_godot_version", map[string]any{
		"project_path":  projectPath,
		"godot_version": testEngineName,
	})
	is.Equal(pinned["success"], true)
	is.Equal(pinned["godot_version"], testEngineName)

	unpinned := callToolOK(t, "unpin_project_from_godot_version", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(unpinned["success"], true)
	is.Equal(unpinned["was_pinned"], true)

	unpinned = callToolOK(t, "unpin_project_from_godot_version", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(unpinned["was_pinned"], false)

	callToolErr(t, "pin_project_to_godot_version", map[string]any{
		"project_path":  projectPath,
		"godot_version": "4.5-stable",
	}, "isn't installed")

	callToolErr(t, "pin_project_to_godot_version", map[string]any{
		"project_path":  "/tmp",
		"godot_version": testEngineName,
	}, "roots")
}
