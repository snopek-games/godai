package mcp

import (
	"path/filepath"
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
	godotPath, _ := cfg["godot_path"].(string)
	is.True(godotPath != "")

	out := callToolOK(t, "set_godai_settings", map[string]any{
		"godot_path": godotPath,
	})
	is.Equal(out["success"], true)

	callToolErr(t, "set_godai_settings", map[string]any{
		"godot_path": "/definitely/not/a/real/godot",
	}, "godot_path")

	callToolErr(t, "set_godai_settings", map[string]any{
		"godot_path": projectPath,
	}, "godot_path")

	callToolErr(t, "set_godai_settings", map[string]any{
		"project_base_path": "/definitely/not/a/real/path",
	}, "project_base_path")

	callToolErr(t, "set_godai_settings", map[string]any{
		"project_base_path": filepath.Join(projectPath, "project.godot"),
	}, "project_base_path")

	cfg = callToolOK(t, "get_godai_settings", nil)
	is.Equal(cfg["godot_path"], godotPath) // unchanged after the failed sets
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

	callToolErr(t, "open_godot_project", map[string]any{
		"project_path": "/tmp",
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
}
