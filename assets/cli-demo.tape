# CLI demo for godai. Run via make-cli-demo.sh, which builds godai,
# prepares a throwaway Godot project, and runs vhs from inside it.
#
# KEEP THIS IN SYNC WITH tests/functional/cli/clidemo_test.go, which runs
# every command shown here against a real editor and checks the output, so
# it's what tells you this recording has gone stale.

Output cli-demo.webm

Require godai
Require fish

# fish gives nice colors
Set Shell fish
Set FontSize 22
Set LineHeight 1.2
Set Width 1200
Set Height 680
Set Padding 20
Set TypingSpeed 40ms
Set WaitTimeout 120s

Hide
Type "set -g fish_color_command brgreen"
Enter
Type "set -g fish_color_option brmagenta"
Enter
Type "set -g fish_color_param normal"
Enter
Type "set -g fish_color_quote yellow"
Enter
Type "set -g fish_color_end brblack"
Enter
Type "set -g fish_color_comment brblack"
Enter
Type "set -g fish_color_valid_path normal"
Enter
Type "set -g fish_autosuggestion_enabled 0"
Enter
Type "clear"
Enter
Show

Type "# Open this project in a headless Godot editor
Sleep 500ms
Enter
Type "godai project open --headless --auto-approve"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor list"
Sleep 500ms
Enter
Wait
Sleep 2s

Type `godai editor-tool get_project_settings "application/config/name"`
Sleep 500ms
Enter
Wait
Sleep 2s

Hide
# For some reason, the "c" in "clear" gets exposed if we don't wait a tiny bit.
# Only on this one, though...
Sleep 50ms
Type "clear"
Enter
Show
Sleep 1s

Type "godai editor-tool create_scene res://demo.tscn --root-node-type Node3D"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool add_node MeshInstance3D \"
Enter
Type "  --property name=Sphere \"
Enter
Type "  --property 'mesh=Object(SphereMesh)'"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool set_node_properties Sphere \"
Enter
Type `  --action "Enlarge the sphere and make it green" \`
Enter
Type "  --property mesh:radius=1.0 \"
Enter
Type "  --property mesh:height=2.0 \"
Enter
Type `  --property 'material_override=Object(StandardMaterial3D,"albedo_color":Color(0, 1, 0, 1))'`
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool get_current_scene_tree"
Sleep 500ms
Enter
Sleep 3s

Hide
Type "clear"
Enter
Show
Sleep 1s

Type "godai editor-tool create_script res://spin.gd \"
Enter
Type "  --content 'extends MeshInstance3D"
Enter
Enter
Type "@export var speed := 1.0"
Enter
Enter
Type "func _process(delta: float) -> void:"
Enter
Type "    rotate_y(speed * delta)"
Enter
Type "'"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool attach_script Sphere res://spin.gd"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool get_current_scene_tree"
Sleep 500ms
Enter
Sleep 3s

Hide
Type "clear"
Enter
Show
Sleep 1s

Type "# An exported variable is just another property"
Sleep 500ms
Enter
Type "godai editor-tool set_node_properties \"
Enter
Type `  --action "Slow the spin" \`
Enter
Type "  --property Sphere:speed=0.25"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool get_node_properties Sphere Sphere:mesh"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool save_scene"
Sleep 500ms
Enter
Wait
Sleep 3s

Hide
Type "clear"
Enter
Show
Sleep 1s

Type "# Anything Godai doesn't have a tool for, can be done with GDScript!"
Sleep 500ms
Enter
Type `godai editor-tool execute_editor_script 'print("hello from inside the editor")'`
Sleep 500ms
Enter
Wait
Sleep 2s

Type "godai editor-tool get_log_messages --count 4"
Sleep 500ms
Enter
Wait
Sleep 2s

Type "# All done! Let's close the Godot editor...
Sleep 500ms
Enter
Type "godai editor close"
Sleep 500ms
Enter
Wait
Sleep 1s
Type "# BYE!!"
Sleep 4s
