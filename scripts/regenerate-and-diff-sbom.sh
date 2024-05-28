#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom.sh
git diff --exit-code --ignore-matching-lines '"timestamp":\s+".+"' cyclonedx.sbom.json
