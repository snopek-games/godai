package stdio

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func findProject(projects []any, path string) map[string]any {
	for _, p := range projects {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if m["project_path"] == path {
			return m
		}
	}
	return nil
}

func TestListProjects(t *testing.T) {
	is := is.New(t)

	structured := callToolOK(t, "list_projects", nil)

	projects, ok := structured["projects"].([]any)
	is.True(ok) // result has a "projects" array

	project := findProject(projects, projectPath)
	is.True(project != nil)                        // the test project is listed
	is.Equal(project["project_name"], projectName) // with its name from project.godot
}

func TestGetSetMcpConfiguration(t *testing.T) {
	is := is.New(t)

	cfg := callToolOK(t, "get_godai_settings", nil)
	_, ok := cfg["godot_version"].(string)
	is.True(ok) // the settings include the Godot version

	out := callToolOK(t, "set_godai_settings", map[string]any{
		"godot_version": testEngineName,
	})
	is.Equal(out["success"], true)

	callToolErr(t, "set_godai_settings", map[string]any{
		"godot_version": "4.5-stable",
	}, "isn't installed")

	callToolErr(t, "set_godai_settings", map[string]any{
		"godot_version": "not a version",
	}, "isn't a Godot version")

	callToolErr(t, "set_godai_settings", map[string]any{
		"project_base_path": "/definitely/not/a/real/path",
	}, "project_base_path")

	callToolErr(t, "set_godai_settings", map[string]any{
		"project_base_path": filepath.Join(projectPath, "project.godot"),
	}, "project_base_path")

	cfg = callToolOK(t, "get_godai_settings", nil)
	is.Equal(cfg["godot_version"], testEngineName) // unchanged after the failed sets
}

func TestOpenGodotProject(t *testing.T) {
	is := is.New(t)

	out := callToolOK(t, "open_godot_project", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(out["success"], true)

	// A second open short-circuits in the server: the editor is already
	// connected, so it returns success without launching anything.
	out = callToolOK(t, "open_godot_project", map[string]any{
		"project_path": projectPath,
	})
	is.Equal(out["success"], true)
	is.Equal(out["already_open"], true)

	callToolErr(t, "open_godot_project", map[string]any{
		"project_path": t.TempDir(),
	}, "")
}

func TestListOpenProjects(t *testing.T) {
	is := is.New(t)

	ensureProjectOpen(t)

	structured := callToolOK(t, "list_open_projects", nil)
	projects, ok := structured["projects"].([]any)
	is.True(ok) // result has a "projects" array

	project := findProject(projects, projectPath)
	is.True(project != nil) // the open project is listed
	is.Equal(project["project_name"], projectName)

	version, _ := project["godot_version"].(string)
	is.True(strings.HasPrefix(version, "4.")) // which Godot it's open in

	// The test editor is launched with a headless display driver.
	is.Equal(project["headless"], true)
}
