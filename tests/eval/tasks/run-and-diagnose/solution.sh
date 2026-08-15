#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run run_project
sleep 2
run get_log_messages
run stop_project

run open_scene --file-path res://main.tscn

run set_node_properties --action 'Rename label to what the script expects' \
	--nodes '{"UI/Score": {"name": "ScoreLabel"}}'

run save_scene
