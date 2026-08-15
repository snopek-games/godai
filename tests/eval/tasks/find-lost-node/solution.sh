#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://main.tscn

run get_current_scene_tree

run get_node_properties \
	--node-paths Coin1 --node-paths Coin2 --node-paths Coin3 --node-paths Coin4

run set_node_properties --action 'Move lost coin back on screen' \
	--nodes '{"Coin3": {"position": "Vector2(400, 300)"}}'

run save_scene
