#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

# $TARGET can be multiple space-separated arguments (e.g. "test:integration
# -ssl=true -auth=true"), so it must be word-split rather than quoted.
# shellcheck disable=SC2086
$GO_EXEC_PREFIX go run build.go -v $TARGET
