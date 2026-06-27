package mcp

import (
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

// findProject looks for a project with the given path in a {"projects": [...]}
// result and returns it.
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

	// get_mcp_configuration reports the godot_path we launched the server with
	// (the headless wrapper).
	cfg := callToolOK(t, "get_mcp_configuration", nil)
	godotPath, _ := cfg["godot_path"].(string)
	is.True(godotPath != "") // godot_path is configured

	// set_mcp_configuration accepts a valid godot_path...
	out := callToolOK(t, "set_mcp_configuration", map[string]any{
		"godot_path": godotPath,
	})
	is.Equal(out["success"], true)

	// ...and rejects a path that doesn't exist...
	callToolErr(t, "set_mcp_configuration", map[string]any{
		"godot_path": "/definitely/not/a/real/godot",
	}, "godot_path")

	// ...as well as one that exists but isn't a regular executable (here a
	// directory), exercising the ValidateGodotExecutable branch.
	callToolErr(t, "set_mcp_configuration", map[string]any{
		"godot_path": projectPath,
	}, "godot_path")

	// project_base_path is validated the same way: a non-existent path...
	callToolErr(t, "set_mcp_configuration", map[string]any{
		"project_base_path": "/definitely/not/a/real/path",
	}, "project_path")

	// ...and a path that exists but isn't a directory are both rejected.
	callToolErr(t, "set_mcp_configuration", map[string]any{
		"project_base_path": filepath.Join(projectPath, "project.godot"),
	}, "project_path")

	cfg = callToolOK(t, "get_mcp_configuration", nil)
	is.Equal(cfg["godot_path"], godotPath) // unchanged after the failed sets
}

func TestOpenGodotProject(t *testing.T) {
	is := is.New(t)

	// First open spawns the editor (the real spawn path); the server installs
	// and enables the addon, launches Godot via --godot-path, and waits for the
	// editor to connect back.
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

	// A project outside the server's allowed roots is rejected.
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
	is.True(project != nil)                        // the open project is listed
	is.Equal(project["project_name"], projectName) // with its name
}
