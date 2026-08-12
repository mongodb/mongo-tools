#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"

if [ -z "$EVG_TRIGGERED_BY_TAG" ]; then
    echo "Cannot regenerate the Augmented SBOM file without a tag"
    exit 1
fi

SBOM_FILE="./ssdlc/$EVG_TRIGGERED_BY_TAG.bom.json"
if [ -z "${branch_name}" ]; then
    KONDUKTO_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
else
    # use default Evergreen expansion for branch name
    KONDUKTO_BRANCH="${branch_name}"
fi

cat <<EOF >silkbomb.env
KONDUKTO_TOKEN=${KONDUKTO_TOKEN}
EOF

./scripts/authenticate-devprod-platforms-ecr.sh

# The arguments to the silkbomb program start at "augment".
#
# shellcheck disable=SC2068 # we don't want to quote `$@`.
podman run \
    --rm \
    --platform linux/amd64 \
    -v "${PWD}":/pwd \
    --env-file silkbomb.env \
    901841024863.dkr.ecr.us-east-1.amazonaws.com/release-infrastructure/silkbomb:2.0 \
    augment \
    --sbom-in /pwd/cyclonedx.sbom.json \
    --repo mongodb/mongo-tools \
    --branch "$KONDUKTO_BRANCH" \
    --sbom-out "/pwd/$SBOM_FILE" \
    $@
