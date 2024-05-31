#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom-lite.sh
git diff --exit-code --ignore-matching-lines '"(timestamp|version)":\s+.+' cyclonedx.sbom.json
