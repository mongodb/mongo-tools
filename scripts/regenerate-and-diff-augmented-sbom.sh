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

./scripts/regenerate-augmented-sbom.sh --no-update-timestamp --no-update-sbom-version
./scripts/diff-sbom.sh "$SBOM"
