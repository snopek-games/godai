#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run read_script --file-path res://player.gd

run write_script --file-path res://player.gd --content 'extends Sprite2D

var speed := 240.0


func _process(delta: float) -> void:
	position.x += speed * delta
'
