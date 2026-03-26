#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

TARBALL_NAME="$1"

SCRIPT_DIR=$(dirname "$0")
# shellcheck disable=SC1091
source "$SCRIPT_DIR/functions.sh"

export PATH="${workdir:?}/.local/bin:$PATH"

mise settings experimental=true

export MISE_DATA_DIR="${workdir:?}/.local/share/mise"

MISE_OUTPUT_FILE=$(mktemp)

# We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install |
    tee "$MISE_OUTPUT_FILE"

retry mise exec node -- npm install --loglevel verbose
