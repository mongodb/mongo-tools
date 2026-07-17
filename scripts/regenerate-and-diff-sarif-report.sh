#!/bin/bash

set -o errexit
set -o pipefail

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

GOSEC_SARIF_REPORT=1 mise exec 'github:houseabsolute/precious' -- precious --quiet lint --all --command gosec

git diff --exit-code SARIF.json
