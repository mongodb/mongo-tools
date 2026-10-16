#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

mise exec 'github:houseabsolute/precious' -- precious lint --all

# Test files must carry a build tag and a filename suffix matching it, so each run only compiles
# the tests it runs and the set a file belongs to is visible from its name.
mise exec go -- go run build.go sa:checkTestFiles
