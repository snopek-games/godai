#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://main.tscn

for i in 1 2 3 4 5; do
	run instantiate_scene --parent-path . --scene-path res://coin.tscn --name "Coin$i"
done

run set_node_properties --action 'Lay out the coin row' --nodes '{
	"Coin1": {"position": "Vector2(100, 200)"},
	"Coin2": {"position": "Vector2(164, 200)"},
	"Coin3": {"position": "Vector2(228, 200)"},
	"Coin4": {"position": "Vector2(292, 200)"},
	"Coin5": {"position": "Vector2(356, 200)"}
}'

run save_scene
