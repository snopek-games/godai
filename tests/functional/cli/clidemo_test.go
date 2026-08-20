package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matryer/is"
)

// Runs godai keeping stdout separate, so checks on a tool's result aren't
// tangled up with the notes it prints to stderr alongside.
func godaiStdout(t *testing.T, args ...string) string {
	t.Helper()

	cmd := command(args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("godai %v: %v\n%s%s", args, err, stderr.String(), stdout.String())
	}
	return stdout.String()
}

// Walks through the CLI demo in assets/cli-demo.tape by running the same
// commands the recording shows, and checks the output it displays. Running the
// real binary means the flags and their values are covered too, not just the
// tools underneath them.
//
// KEEP THIS IN SYNC WITH THE TAPE: when a command or its output changes, this
// test should be what tells you the tape needs an edit (and the demo a
// re-recording via scripts/make-cli-demo.sh).
//
// The tape's `godai project open` and closing `godai editor close` are the
// only commands not repeated here: the harness owns the editor these commands
// talk to, and other tests still need it running.
func TestCliDemoTape(t *testing.T) {
	is := is.New(t)

	// godai editor list
	openProjects := godai(t, "editor", "list")
	is.True(strings.Contains(openProjects, projectName))
	is.True(strings.Contains(openProjects, "GODOT")) // with the version it's open in

	// godai editor-tool get_project_settings application/config/name
	name := godaiStdout(t, "editor-tool", "get_project_settings",
		"application/config/name")
	is.Equal(name, "application/config/name = "+projectName+"\n")

	// godai editor-tool create_scene res://demo.tscn --root-node-type Node3D
	//
	// A plain success prints nothing, like the demo shows.
	created := godaiStdout(t, "editor-tool", "create_scene",
		"res://demo.tscn",
		"--root-node-type", "Node3D")
	is.Equal(created, "")

	// godai editor-tool add_node MeshInstance3D \
	//   --property name=Sphere \
	//   --property 'mesh=Object(SphereMesh)'
	added := godaiStdout(t, "editor-tool", "add_node",
		"MeshInstance3D",
		"--property", "name=Sphere",
		"--property", "mesh=Object(SphereMesh)")
	is.Equal(added, "node_path: Sphere\n")

	// godai editor-tool set_node_properties Sphere \
	//   --action "Enlarge the sphere and make it green" \
	//   --property mesh:radius=1.0 \
	//   --property mesh:height=2.0 \
	//   --property 'material_override=Object(StandardMaterial3D,"albedo_color":Color(0, 1, 0, 1))'
	enlarged := godaiStdout(t, "editor-tool", "set_node_properties",
		"Sphere",
		"--action", "Enlarge the sphere and make it green",
		"--property", "mesh:radius=1.0",
		"--property", "mesh:height=2.0",
		"--property", `material_override=Object(StandardMaterial3D,"albedo_color":Color(0, 1, 0, 1))`)
	is.Equal(enlarged, "")

	// godai editor-tool get_current_scene_tree
	tree := godaiStdout(t, "editor-tool", "get_current_scene_tree")
	is.Equal(tree, "Demo (Node3D)\n  Sphere (MeshInstance3D)\n")

	// godai editor-tool create_script res://spin.gd --content '...'
	const spinScript = `extends MeshInstance3D

@export var speed := 1.0

func _process(delta: float) -> void:
    rotate_y(speed * delta)
`
	godaiStdout(t, "editor-tool", "create_script",
		"res://spin.gd",
		"--content", spinScript)

	// godai editor-tool attach_script Sphere res://spin.gd
	godaiStdout(t, "editor-tool", "attach_script", "Sphere", "res://spin.gd")

	// godai editor-tool get_current_scene_tree
	//
	// The second look in the demo, now showing the attached script.
	tree = godaiStdout(t, "editor-tool", "get_current_scene_tree")
	is.Equal(tree, "Demo (Node3D)\n  Sphere (MeshInstance3D) res://spin.gd\n")

	// godai editor-tool set_node_properties \
	//   --action "Slow the spin" \
	//   --property Sphere:speed=0.25
	godaiStdout(t, "editor-tool", "set_node_properties",
		"--action", "Slow the spin",
		"--property", "Sphere:speed=0.25")

	// godai editor-tool get_node_properties Sphere Sphere:mesh
	props := godaiStdout(t, "editor-tool", "get_node_properties",
		"Sphere", "Sphere:mesh")
	is.Equal(props, `Sphere:
  material_override = Object(StandardMaterial3D)
  mesh = Object(SphereMesh)
  name = Sphere
  script = Resource("res://spin.gd")
  speed = 0.25
Sphere:mesh:
  height = 2.0
  radius = 1.0
`)

	// godai editor-tool save_scene
	saved := godaiStdout(t, "editor-tool", "save_scene")
	is.Equal(saved, "scene_path: res://demo.tscn\n")

	// godai editor-tool execute_editor_script 'print("hello from inside the editor")'
	hello := godaiStdout(t, "editor-tool", "execute_editor_script",
		`print("hello from inside the editor")`)
	is.True(strings.Contains(hello, "hello from inside the editor"))

	// godai editor-tool get_log_messages --count 4
	logged := godaiStdout(t, "editor-tool", "get_log_messages", "--count", "4")
	is.True(strings.Contains(logged, "hello from inside the editor"))
}
