#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

# The eval_stale plugin overwrites the image shortly after startup; a model
# spends longer than that thinking, but the oracle has to wait explicitly.
for _ in $(seq 60); do
	[ -f "$project/.godot/eval_stale_done" ] && break
	sleep 0.5
done

run reimport --file-paths res://sprites/tileset.png
