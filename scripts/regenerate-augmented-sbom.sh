#!/bin/bash

set -e
set -x
set -o pipefail

TAG="$EVG_TRIGGERED_BY_TAG"
if [ -z "$TAG" ]; then
    echo "Cannot regenerate the Augmented SBOM file without a tag"
    exit 0
fi

SBOM_FILE="./ssdlc/$TAG.bom.json"

cat <<EOF >silkbomb.env
KONDUKTO_TOKEN=${KONDUKTO_TOKEN}
EOF

# The arguments to the silkbomb program start at "augment".
#
# shellcheck disable=SC2068 # we don't want to quote `$@`.
podman run \
    --rm \
    --platform linux/amd64 \
    -v "${PWD}":/pwd \
    --env-file silkbomb.env \
    artifactory.corp.mongodb.com/release-tools-container-registry-public-local/silkbomb:2.0 \
    augment \
    --sbom-in /pwd/cyclonedx.sbom.json \
    --repo mongodb/mongo-tools \
    --branch ${branch_name} \
    --sbom-out "/pwd/$SBOM_FILE" \
    $@
