#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom-lite.sh --no-update-timestamp --no-update-sbom-version
./scripts/diff-sbom.sh cyclonedx.sbom.json
