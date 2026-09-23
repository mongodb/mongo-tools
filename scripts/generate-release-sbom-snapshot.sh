#!/bin/bash

# Generates the historical SBOM + SARIF snapshot for a release: writes ssdlc/<tag>.bom.json (a
# fresh SBOM via scripts/generate-sbom.sh) and ssdlc/<tag>.sarif.json (a fresh gosec SARIF report).
# Neither file is checked in continuously -- there's no developer-facing "keep this up to date"
# burden for either the SBOM or the SARIF report; both are generated fresh only when a release is
# actually being cut. See RELEASE.md's "Create the SBOM and SARIF Report Files for the Upcoming
# Release" section -- a human runs this before tagging a release, then commits the two resulting
# files.

set -e
set -o pipefail
set -x

if [ $# -ne 1 ]; then
    echo "usage: generate-release-sbom-snapshot.sh <tag>" >&2
    exit 1
fi
TAG="$1"

./scripts/generate-sbom.sh --output "ssdlc/${TAG}.bom.json"

# etc/gosec-wrapper.sh always writes to ./SARIF.json (not configurable), so generate at the repo
# root as usual, then move it into place -- SARIF.json itself is gitignored and never committed.
GOSEC_SARIF_REPORT=1 mise exec 'github:houseabsolute/precious' -- precious --quiet lint --all --command gosec
mv SARIF.json "ssdlc/${TAG}.sarif.json"
