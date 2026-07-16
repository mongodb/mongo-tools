#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"

$GO_EXEC_PREFIX go run release/release.go build-archive
$GO_EXEC_PREFIX go run release/release.go build-packages
