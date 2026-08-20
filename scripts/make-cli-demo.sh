#!/usr/bin/env bash
#
# Build godai and record the CLI demo video with vhs, producing
# assets/cli-demo.webm (for the website) and assets/cli-demo.gif
# (for the README, since GitHub won't inline repo-hosted video files).
#
# Requires: go, vhs (https://github.com/charmbracelet/vhs), ffmpeg, and fish
# (which vhs records in, for its command syntax highlighting). If gifsicle
# is installed, it's used to shrink the gif further.

set -euo pipefail

for tool in go vhs ffmpeg fish; do
	if ! command -v "$tool" >/dev/null; then
		echo "error: $tool is required but not on the PATH" >&2
		exit 1
	fi
done

GODOT_VERSION="${GODOT_VERSION:-4.6}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
TAPE="$REPO_DIR/assets/cli-demo.tape"
OUTPUT="$REPO_DIR/assets/cli-demo.webm"
GIF_OUTPUT="$REPO_DIR/assets/cli-demo.gif"

WORKDIR="$(mktemp -d)"
# A stable, readable path, because it appears in the recording.
DEMO_PROJECT="/tmp/godai-demo"

cleanup() {
	godai editor close --skip-save "$DEMO_PROJECT" >/dev/null 2>&1 || true
	rm -rf "$WORKDIR" "$DEMO_PROJECT"
}
trap cleanup EXIT

echo "Building godai ..."
(cd "$REPO_DIR" && go build -o "$WORKDIR/bin/godai" ./cmd/godai)
export PATH="$WORKDIR/bin:$PATH"

if ! godai engine list | grep -qF "$GODOT_VERSION"; then
	echo "Installing Godot $GODOT_VERSION ..."
	godai engine install "$GODOT_VERSION"
fi

rm -rf "$DEMO_PROJECT"
mkdir "$DEMO_PROJECT"
cat > "$DEMO_PROJECT/project.godot" <<EOF
config_version=5

[application]

config/name="Godai Demo"
EOF
(cd "$DEMO_PROJECT" && godai project pin-engine "$GODOT_VERSION")

echo "Recording demo ..."
(cd "$DEMO_PROJECT" && vhs "$TAPE" --output "$OUTPUT")

echo "Converting to gif ..."
# 32 colors with no dither is enough for terminal output, and rectangle
# diffing re-encodes only the parts of each frame that changed.
ffmpeg -v error -y -i "$OUTPUT" -vf "fps=10,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=stats_mode=diff:max_colors=32[p];[s1][p]paletteuse=dither=none:diff_mode=rectangle" -loop 0 "$GIF_OUTPUT"
if command -v gifsicle >/dev/null; then
	gifsicle -O3 --lossy=80 -b "$GIF_OUTPUT"
fi

echo "Wrote $OUTPUT and $GIF_OUTPUT"
