package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

// Walks through the example in the README's "CLI mode" section by running the
// same commands the README shows, and checks the output it documents. Running
// the real binary means the flags and their values are covered too, not just
// the tools underneath them.
//
// KEEP THIS IN SYNC WITH THE README: when a command or its output changes,
// this test should be what tells you the README needs an edit.
//
// The commands here drop the `-p ~/games/platformer` the README uses, since
// these run from inside the project, exactly as the README says you can.
func TestReadmeExample(t *testing.T) {
	is := is.New(t)

	// godai editor list
	openProjects := godai(t, "editor", "list")
	is.True(strings.Contains(openProjects, projectName))

	// godai editor-tool --help
	tools := godai(t, "editor-tool", "--help")
	is.True(strings.Contains(tools, "set_node_properties"))

	// godai editor-tool get_project_settings --help
	help := godai(t, "editor-tool", "get_project_settings", "--help")
	is.True(strings.Contains(help, "--names"))

	// godai editor-tool get_project_settings --names application/config/name
	settings := decode(t, godai(t, "editor-tool", "get_project_settings",
		"--names", "application/config/name"))
	names, _ := settings["settings"].(map[string]any)
	is.Equal(names["application/config/name"], projectName)

	// godai editor-tool create_scene \
	//   --file-path res://test.tscn \
	//   --root-node-type Node3D
	created := decode(t, godai(t, "editor-tool", "create_scene",
		"--file-path", "res://test.tscn",
		"--root-node-type", "Node3D"))
	is.Equal(created["success"], true)

	// godai editor-tool add_node \
	//   --parent-path . \
	//   --node-type MeshInstance3D \
	//   --properties name=Sphere \
	//   --properties 'mesh=Object(SphereMesh)'
	added := decode(t, godai(t, "editor-tool", "add_node",
		"--parent-path", ".",
		"--node-type", "MeshInstance3D",
		"--properties", "name=Sphere",
		"--properties", "mesh=Object(SphereMesh)"))
	is.Equal(added["node_path"], "Sphere")

	// godai editor-tool set_node_properties \
	//   --action "Make the sphere green" \
	//   --nodes '{"Sphere": {"material_override": "Object(StandardMaterial3D,\"albedo_color\":Color(0, 1, 0, 1))"}}'
	green := decode(t, godai(t, "editor-tool", "set_node_properties",
		"--action", "Make the sphere green",
		"--nodes", `{"Sphere": {"material_override": "Object(StandardMaterial3D,\"albedo_color\":Color(0, 1, 0, 1))"}}`))
	is.Equal(green["success"], true)

	// godai editor-tool set_node_properties \
	//   --action "Enlarge the sphere" \
	//   --nodes '{"Sphere": {"mesh:radius": "1.0", "mesh:height": "2.0"}}'
	enlarged := decode(t, godai(t, "editor-tool", "set_node_properties",
		"--action", "Enlarge the sphere",
		"--nodes", `{"Sphere": {"mesh:radius": "1.0", "mesh:height": "2.0"}}`))
	is.Equal(enlarged["success"], true)

	// godai editor-tool create_script \
	//   --file-path res://spin.gd \
	//   --content '...'
	const spinScript = `extends MeshInstance3D

@export var speed := 1.0

func _process(delta: float) -> void:
    rotate_y(speed * delta)
`
	godai(t, "editor-tool", "create_script",
		"--file-path", "res://spin.gd",
		"--content", spinScript)

	// godai editor-tool attach_script \
	//   --node-path Sphere \
	//   --script-path res://spin.gd
	godai(t, "editor-tool", "attach_script",
		"--node-path", "Sphere",
		"--script-path", "res://spin.gd")

	// godai editor-tool set_node_properties \
	//   --action "Slow the spin" \
	//   --nodes '{"Sphere": {"speed": "0.25"}}'
	godai(t, "editor-tool", "set_node_properties",
		"--action", "Slow the spin",
		"--nodes", `{"Sphere": {"speed": "0.25"}}`)

	// godai editor-tool get_node_properties \
	//   --node-paths Sphere \
	//   --node-paths Sphere:mesh \
	//   --node-paths Sphere:material_override
	props := decode(t, godai(t, "editor-tool", "get_node_properties",
		"--node-paths", "Sphere",
		"--node-paths", "Sphere:mesh",
		"--node-paths", "Sphere:material_override"))
	is.Equal(props, readmeJSON(t, `{
  "Sphere": {
    "material_override": "Object(StandardMaterial3D)",
    "mesh": "Object(SphereMesh)",
    "name": "Sphere",
    "script": "Resource(\"res://spin.gd\")",
    "speed": "0.25"
  },
  "Sphere:material_override": {
    "albedo_color": "Color(0, 1, 0, 1)"
  },
  "Sphere:mesh": {
    "height": "2.0",
    "radius": "1.0"
  }
}`))

	// godai editor-tool get_current_scene_tree
	tree := decode(t, godai(t, "editor-tool", "get_current_scene_tree"))
	is.Equal(tree, readmeJSON(t, `{
  "children": [
    {
      "name": "Sphere",
      "path": "Sphere",
      "script": "res://spin.gd",
      "type": "MeshInstance3D"
    }
  ],
  "name": "Test",
  "path": ".",
  "type": "Node3D"
}`))

	// godai editor-tool save_scene
	saved := decode(t, godai(t, "editor-tool", "save_scene"))
	is.Equal(saved["scene_path"], "res://test.tscn")

	// godai editor-tool read_script --file-path res://spin.gd
	read := decode(t, godai(t, "editor-tool", "read_script",
		"--file-path", "res://spin.gd"))
	is.Equal(read["content"], spinScript)

	// godai editor-tool write_script --file-path res://spin.gd --content '...'
	// Only works because of the read above: each command is its own connection
	// to the editor, so the editor has to remember the read across both.
	rewritten := strings.Replace(spinScript, "speed * delta", "speed * delta * 2.0", 1)
	written := decode(t, godai(t, "editor-tool", "write_script",
		"--file-path", "res://spin.gd",
		"--content", rewritten))
	is.Equal(written["success"], true)

	// ... and the README's claim about what write_script refuses.
	godai(t, "editor-tool", "execute_editor_script", "--code",
		`var f = FileAccess.open("res://spin.gd", FileAccess.WRITE)
f.store_string("extends MeshInstance3D\n")
f.close()
EditorInterface.get_resource_filesystem().update_file("res://spin.gd")
return OK`)
	refused := godaiErr(t, "editor-tool", "write_script",
		"--file-path", "res://spin.gd",
		"--content", spinScript)
	is.True(strings.Contains(refused, "has changed since you last read it"))

	// godai editor-tool get_log_messages
	logged := decode(t, godai(t, "editor-tool", "get_log_messages", "--count", "5"))
	_, hasMessages := logged["messages"]
	is.True(hasMessages)
}

// Parses a JSON block copied out of the README, so the expected output here can
// be the documented output, character for character.
func readmeJSON(t *testing.T, blob string) map[string]any {
	t.Helper()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		t.Fatalf("parsing the README's JSON: %v", err)
	}
	return parsed
}
