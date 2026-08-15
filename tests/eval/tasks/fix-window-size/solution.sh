#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run get_project_settings \
	--names display/window/size/viewport_width \
	--names display/window/size/viewport_height

run set_project_settings \
	--settings 'display/window/size/viewport_width=1280' \
	--settings 'display/window/size/viewport_height=720'
