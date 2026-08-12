#!/bin/sh
# Build the godai CLI and install it as `godai-dev` for manual testing, so a
# development build can sit on PATH alongside a released `godai`.
#
# Usage:
#   scripts/install-dev.sh          # install to $HOME/bin/godai-dev
#   GODAI_DEV_BIN=/path/to/godai-dev scripts/install-dev.sh
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

DEST="${GODAI_DEV_BIN:-$HOME/bin/godai-dev}"
DEST_DIR="$(dirname "$DEST")"

mkdir -p "$DEST_DIR"

# Build beside the destination and rename over it: writing straight to $DEST
# fails with ETXTBSY while an older godai-dev is still running.
TMP_OUT="$(mktemp "$DEST.XXXXXX")"
trap 'rm -f "$TMP_OUT"' EXIT

echo "Building ./cmd/godai/ ..."
go build -o "$TMP_OUT" ./cmd/godai/

chmod 755 "$TMP_OUT"
mv "$TMP_OUT" "$DEST"
trap - EXIT

echo "Installed $DEST"
"$DEST" --version

case ":${PATH}:" in
	*":$DEST_DIR:"*) ;;
	*) echo "Note: $DEST_DIR is not on your PATH." ;;
esac
