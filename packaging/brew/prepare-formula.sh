#!/bin/sh
# Renders godai.rb.tmpl into a Homebrew formula, filling in the version and
# the sha256 of each platform's release zip from a `sha256sum`-format
# checksums file (the one the release CI job generates).
#
# Usage: packaging/brew/prepare-formula.sh <version> <checksums-file> <output-file>

set -eu

if [ $# -ne 3 ]; then
    echo "Usage: $0 <version> <checksums-file> <output-file>" >&2
    exit 1
fi

VERSION="$1"
CHECKSUMS="$2"
OUT="$3"

BREW_DIR="$(dirname "$0")"

sha_for() {
    fn="godai-cli-$1-v${VERSION}.zip"
    sha="$(awk -v fn="$fn" '$2 == fn { print $1 }' "$CHECKSUMS")"
    if [ -z "$sha" ]; then
        echo "ERROR: no checksum for $fn in $CHECKSUMS" >&2
        exit 1
    fi
    echo "$sha"
}

SHA256_MACOS_ARM64="$(sha_for macos-arm64)"
SHA256_MACOS_X86_64="$(sha_for macos-x86_64)"
SHA256_LINUX_ARM64="$(sha_for linux-arm64)"
SHA256_LINUX_X86_64="$(sha_for linux-x86_64)"

mkdir -p "$(dirname "$OUT")"
sed \
    -e "s/@VERSION@/${VERSION}/g" \
    -e "s/@SHA256_MACOS_ARM64@/${SHA256_MACOS_ARM64}/g" \
    -e "s/@SHA256_MACOS_X86_64@/${SHA256_MACOS_X86_64}/g" \
    -e "s/@SHA256_LINUX_ARM64@/${SHA256_LINUX_ARM64}/g" \
    -e "s/@SHA256_LINUX_X86_64@/${SHA256_LINUX_X86_64}/g" \
    "$BREW_DIR/godai.rb.tmpl" > "$OUT"

echo "Wrote $OUT (version ${VERSION})"
