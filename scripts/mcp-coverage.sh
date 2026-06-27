#!/bin/sh
# Produce a combined test-coverage report for the Go MCP server (./mcp/server).
#
# Coverage from two sources is merged:
#   - the in-package unit tests (go test ./mcp/server/), and
#   - the functional tests (./tests/functional/mcp), which build the godai-mcp
#     binary with -cover and run it as a subprocess. The subprocess writes its
#     coverage data to GOCOVERDIR, enabled here via GODAI_COVERDIR.
#
# Both runs emit coverage in Go's binary "covdata" format, which `go tool
# covdata` then merges into a single percentage / per-function / profile report.
#
# Requires a Godot binary on PATH (or via $GODOT) for the functional tests; if
# none is found those tests skip and the report reflects unit coverage only.
#
# Usage:
#   scripts/mcp-coverage.sh            # print merged per-function + total
#   scripts/mcp-coverage.sh -o cov.out # also write a merged text profile
#                                      # (open with: go tool cover -html=cov.out)
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PROFILE_OUT=""
if [ "${1:-}" = "-o" ]; then
	PROFILE_OUT="${2:?-o requires a file path}"
fi

COVPKG="gitlab.com/snopek-games/godai/mcp/..."
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
UNIT_DIR="$WORK/unit"
FUNC_DIR="$WORK/func"
mkdir -p "$UNIT_DIR" "$FUNC_DIR"

echo ">> unit tests (./mcp/server)"
go test -count=1 -cover -coverpkg="$COVPKG" ./mcp/server/ \
	-args -test.gocoverdir="$UNIT_DIR"

echo ">> functional tests (./tests/functional/mcp)"
GODAI_COVERDIR="$FUNC_DIR" go test -count=1 ./tests/functional/mcp/

# If the functional tests skipped (no Godot), FUNC_DIR has no covmeta file;
# merging an empty dir is fine, but report it so the number isn't misread.
if ! ls "$FUNC_DIR"/covmeta.* >/dev/null 2>&1; then
	echo ">> NOTE: no functional coverage emitted (Godot missing or tests skipped)"
fi

INPUTS="$UNIT_DIR,$FUNC_DIR"

echo
echo "=== merged coverage: $COVPKG ==="
go tool covdata percent -i="$INPUTS"

echo
echo "=== per-function (mcp/server) ==="
go tool covdata textfmt -i="$INPUTS" -o="$WORK/merged.txt"
go tool cover -func="$WORK/merged.txt" | grep -E "mcp/server|^total"

if [ -n "$PROFILE_OUT" ]; then
	cp "$WORK/merged.txt" "$PROFILE_OUT"
	echo
	echo ">> merged profile written to $PROFILE_OUT (go tool cover -html=$PROFILE_OUT)"
fi
