#!/bin/sh
# Run godai-eval in the image build.sh makes; all arguments go to godai-eval.
# The repo is mounted at /work, so tasks are read from and results written to
# the working tree — but godai and godai-eval run from the image, so rebuild
# it after changing Go code.
set -eu

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../../.." && pwd)"

TTY=""
if [ -t 0 ]; then
	TTY="-t"
fi

exec docker run --rm -i $TTY --init \
	--user "$(id -u):$(id -g)" \
	-e ANTHROPIC_API_KEY \
	-e CLAUDE_CODE_OAUTH_TOKEN \
	-v "$REPO_ROOT:/work" \
	godai-eval "$@"
