#!/bin/bash

set -e
set -x

./scripts/regenerate-sbom-lite.sh
./scripts/diff-sbom.sh cyclonedx.sbom.json
