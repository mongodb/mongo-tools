#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom-lite.sh --no-update-timestamp --no-update-sbom-version
git diff --exit-code cyclonedx.sbom.json
