#!/usr/bin/env bash
# Oracle: solves the task with godai directly, no model involved, to prove the
# task is still solvable.

set -euo pipefail

godai=${1:?usage: solution.sh <godai-binary> <project-dir>}
project=${2:?usage: solution.sh <godai-binary> <project-dir>}

minigame="$project/minigame"

"$godai" --root "$project" --no-input --no-auto-install \
	project open "$minigame" --headless --auto-approve

"$godai" --root "$project" --no-input --no-auto-install \
	editor-tool set_project_settings -p "$minigame" \
	--settings 'application/config/name=Bonus Round'
