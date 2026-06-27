#!/bin/bash

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd -P)"

cd "$SCRIPT_DIR"

exec go run ./cmd/godai-mcp/ "$@"

