#!/bin/bash

set -e
set -x
set -o pipefail

TAG="$EVG_TRIGGERED_BY_TAG"
if [ -z "$TAG" ]; then
    echo "Cannot regenerate the Augmented SBOM file without a tag"
    exit 1
fi

SBOM_FILE="./ssdlc/$TAG.bom.json"
if [ -z "${branch_name}" ]; then
    KONDUKTO_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
else
    # use default Evergreen expansion for branch name
    KONDUKTO_BRANCH="${branch_name}"
fi

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
    --branch "$KONDUKTO_BRANCH" \
    --sbom-out "/pwd/$SBOM_FILE" \
    $@
