#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://main.tscn

run execute_editor_script --code 'var orbs := EditorInterface.get_edited_scene_root().get_node("Orbs")
var count := orbs.get_child_count()
var undo_redo := EditorInterface.get_editor_undo_redo()
undo_redo.create_action("Arrange orbs in a circle (Godai)")
for i in count:
	var orb: Node2D = orbs.get_child(i)
	var angle := TAU * i / count
	var target := Vector2(400, 300) + Vector2(cos(angle), sin(angle)) * 200.0
	undo_redo.add_do_property(orb, "position", target)
	undo_redo.add_undo_property(orb, "position", orb.position)
undo_redo.commit_action()'

run save_scene
