#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.
#
#   XDG_CACHE_HOME=<work>/cache solution.sh <godai-binary> <project-dir>

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

run() { "$godai" --root "$project" --no-input --no-auto-install editor-tool "$@" -p "$project"; }

run open_scene --file-path res://main.tscn

run add_node --parent-path . --node-type Sprite2D \
	--properties name=Player \
	--properties 'position=Vector2(100, 50)'

run create_script --file-path res://player.gd --content 'extends Sprite2D

@export var speed := 200.0

func _process(delta: float) -> void:
	position.x += speed * delta
'

run attach_script --node-path Player --script-path res://player.gd
run save_scene
