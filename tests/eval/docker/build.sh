#!/bin/sh
# Build the godai-eval image; extra arguments are passed to `docker build`.
set -eu

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"

exec docker build \
	--file "$SCRIPT_DIR/Dockerfile" \
	--tag godai-eval \
	"$@" \
	"$SCRIPT_DIR/../../.."
