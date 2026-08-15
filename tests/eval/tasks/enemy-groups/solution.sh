#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://arena.tscn

run get_node_groups \
	--node-paths Enemy1 --node-paths Enemy2 --node-paths Enemy3 --node-paths Player

run add_to_group --node-path Enemy3 --groups enemies
run remove_from_group --node-path Player --groups enemies

run save_scene
