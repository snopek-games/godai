#!/bin/sh
# Download a Godot editor and put it on PATH, for CI images that don't ship one.
set -eu

VERSION="${GODOT_VERSION:?set GODOT_VERSION, for example 4.6.3}"
DEST="${GODOT_BIN:-/usr/local/bin/godot}"

RELEASE="Godot_v${VERSION}-stable_linux.x86_64"
URL="https://github.com/godotengine/godot/releases/download/${VERSION}-stable/${RELEASE}.zip"

# Skipped where we couldn't install them anyway, which is also where they're
# likely to be there already.
if [ "$(id -u)" = 0 ] && command -v apt-get >/dev/null 2>&1; then
	apt-get update
	apt-get install -y --no-install-recommends unzip libfontconfig1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "Installing Godot ${VERSION} to ${DEST} ..."
wget -q -O "$TMP/godot.zip" "$URL"
unzip -q "$TMP/godot.zip" -d "$TMP"

mv "$TMP/$RELEASE" "$DEST"
chmod 755 "$DEST"

"$DEST" --version
