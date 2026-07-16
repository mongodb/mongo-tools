#!/bin/bash

set -e
set -x

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

./scripts/regenerate-sbom-lite.sh --no-update-timestamp --no-update-sbom-version
git diff --exit-code cyclonedx.sbom.json
