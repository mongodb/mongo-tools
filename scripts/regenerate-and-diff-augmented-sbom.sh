#!/bin/bash

set -e
set -x

TAG="$EVG_TRIGGERED_BY_TAG"
if [ -z "$TAG" ]; then
    echo "Cannot regenerate the Augmented SBOM file without a tag"
    exit 0
fi

SBOM="ssdlc/$TAG.bom.json"
if [ ! -f "$SBOM" ]; then
    echo "The $SBOM does not exist at all"
    exit 1
fi

./scripts/regenerate-augmented-sbom.sh

# TODO (TOOLS-3621): Check the entire file once DEVPROD-9074 is fixed.
git diff --exit-code --ignore-matching-lines='timestamp' --ignore-matching-lines='bom-ref' "$SBOM"
