#!/bin/sh
# Installs the godai CLI on Linux or macOS:
#
#   curl -fsSL https://godai.sh/install | bash
#
# (or, equivalently, the raw file from the repository:
# https://gitlab.com/snopek-games/godai/-/raw/main/packaging/install/install.sh)
#
# Configuration (environment variables):
#   GODAI_VERSION         version to install, without the leading "v" (default: latest release)
#   GODAI_INSTALL_DIR     where to put the binary (default: ~/.local/bin)
#   GODAI_NO_MODIFY_PATH  if non-empty, never edit shell config files to add
#                         the install dir to PATH
#   GODAI_DOWNLOAD_BASE   override the release download base URL (used by CI
#                         to test this script against a local server)

set -eu

PROJECT_API="https://gitlab.com/api/v4/projects/snopek-games%2Fgodai"
BASE_URL="${GODAI_DOWNLOAD_BASE:-$PROJECT_API/packages/generic/release-packages}"
INSTALL_DIR="${GODAI_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${GODAI_VERSION:-}"

err() {
    echo "ERROR: $*" >&2
    exit 1
}

command -v curl > /dev/null || err "curl is required"
command -v unzip > /dev/null || err "unzip is required"

case "$(uname -s)" in
    Linux) PLATFORM="linux" ;;
    Darwin) PLATFORM="macos" ;;
    *) err "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
    x86_64 | amd64) ARCH="x86_64" ;;
    arm64 | aarch64) ARCH="arm64" ;;
    *) err "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$VERSION" ]; then
    VERSION="$(curl -fsSL "$PROJECT_API/releases/permalink/latest" \
        | sed -n 's/.*"tag_name":"v\([^"]*\)".*/\1/p')"
    [ -n "$VERSION" ] || err "couldn't determine the latest version; set GODAI_VERSION to pick one explicitly"
fi

ASSET="godai-cli-${PLATFORM}-${ARCH}-v${VERSION}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading godai v${VERSION} (${PLATFORM}-${ARCH})..."
curl -fsSL -o "$TMP_DIR/$ASSET.zip" "$BASE_URL/v${VERSION}/$ASSET.zip"
curl -fsSL -o "$TMP_DIR/checksums.txt" "$BASE_URL/v${VERSION}/checksums-v${VERSION}.txt"

EXPECTED="$(awk -v fn="$ASSET.zip" '$2 == fn { print $1 }' "$TMP_DIR/checksums.txt")"
[ -n "$EXPECTED" ] || err "no checksum for $ASSET.zip in the release's checksums file"
if command -v sha256sum > /dev/null; then
    ACTUAL="$(sha256sum "$TMP_DIR/$ASSET.zip" | awk '{ print $1 }')"
else
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$ASSET.zip" | awk '{ print $1 }')"
fi
[ "$ACTUAL" = "$EXPECTED" ] || err "checksum mismatch for $ASSET.zip (expected $EXPECTED, got $ACTUAL)"

unzip -q "$TMP_DIR/$ASSET.zip" -d "$TMP_DIR"
mkdir -p "$INSTALL_DIR"
install -m 755 "$TMP_DIR/$ASSET/godai" "$INSTALL_DIR/godai"

echo "Installed godai v${VERSION} to $INSTALL_DIR/godai"

case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        echo "Run \"godai --help\" to get started, and \"godai self-update\" to update later."
        exit 0
        ;;
esac

EXPORT_LINE="export PATH=\"$INSTALL_DIR:\$PATH\""
RC=""
if [ -z "${GODAI_NO_MODIFY_PATH:-}" ]; then
    case "$(basename "${SHELL:-sh}")" in
        zsh) RC="$HOME/.zshrc" ;;
        bash)
            # macOS terminals start login shells, which skip .bashrc.
            if [ "$PLATFORM" = "macos" ]; then
                RC="$HOME/.bash_profile"
            else
                RC="$HOME/.bashrc"
            fi
            ;;
    esac
fi

if [ -n "$RC" ]; then
    if [ ! -f "$RC" ] || ! grep -qxF "$EXPORT_LINE" "$RC"; then
        printf '\n%s\n' "$EXPORT_LINE" >> "$RC"
    fi
    echo "Added $INSTALL_DIR to your PATH in $RC - open a new terminal for it to take effect."
else
    echo "NOTE: $INSTALL_DIR is not on your PATH. Add it with:"
    echo "  $EXPORT_LINE"
fi
echo "Then run \"godai --help\" to get started, and \"godai self-update\" to update later."
