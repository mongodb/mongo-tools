#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom-lite.sh --no-update-timestamp --no-update-sbom-version

# TODO (TOOLS-3621): Check the entire file once DEVPROD-9074 is fixed.
git diff --exit-code cyclonedx.sbom.json --ignore-matching-lines='timestamp' --ignore-matching-lines='bom-ref'
