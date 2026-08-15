#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run create_resource --file-path res://sky_gradient.tres --resource-type Gradient \
	--properties 'colors=PackedColorArray(0.05, 0.15, 0.5, 1, 1, 0.8, 0.6, 1)'
