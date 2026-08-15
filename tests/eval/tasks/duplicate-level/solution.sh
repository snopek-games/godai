#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://level_1.tscn

run save_scene_as --file-path res://level_2.tscn

run set_node_properties --action 'Rename level root' \
	--nodes '{".": {"name": "Level2"}}'

run save_scene
