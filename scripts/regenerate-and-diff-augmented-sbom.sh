#!/bin/bash

set -e
set -x

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"

SBOM="ssdlc/$EVG_TRIGGERED_BY_TAG.bom.json"
if [ ! -f "$SBOM" ]; then
    echo "The $SBOM does not exist at all"
    exit 1
fi

./scripts/regenerate-augmented-sbom.sh

# TODO (TOOLS-3621): Check the entire file once DEVPROD-9074 is fixed.
git diff --exit-code --ignore-matching-lines='timestamp' --ignore-matching-lines='bom-ref' "$SBOM"
