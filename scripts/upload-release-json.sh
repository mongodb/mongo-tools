#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"

# Intentionally hardcoded rather than using $GO_EXEC_PREFIX: this preserves
# today's exact behavior (the pre-migration script also hardcodes
# "mise exec go --" here), not an oversight.
mise exec go -- go run release/release.go upload-json
