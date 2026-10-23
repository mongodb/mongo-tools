#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

# Regenerate the checked-in passthrough Evergreen config. The generator formats the file with
# precious, so if the checked-in copy is up to date this leaves the working tree clean.
mise exec go -- go run ./dev-tools/mongodump-task-gen

git diff --exit-code -- evergreen/config/mongodump-passthrough.yml
