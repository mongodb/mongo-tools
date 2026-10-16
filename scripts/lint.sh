#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

mise exec 'github:houseabsolute/precious' -- precious lint --all

# Server-facing tests must have a build tag so they only compile for the test run that runs them.
mise exec go -- go run build.go sa:checkTestTags
