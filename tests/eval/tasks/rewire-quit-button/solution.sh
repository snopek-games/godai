#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://menu.tscn

run disconnect_signal --from-node QuitButton --signal pressed \
	--to-node . --method _on_start_button_pressed

run connect_signal --from-node QuitButton --signal pressed \
	--to-node . --method _on_quit_button_pressed

run save_scene
