#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run create_scene --file-path res://title.tscn --root-node-type Control

run add_node --parent-path . --node-type Label \
	--properties name=Title \
	--properties 'text=Space Miner'

run save_scene

run set_project_settings --settings 'application/run/main_scene=res://title.tscn'
