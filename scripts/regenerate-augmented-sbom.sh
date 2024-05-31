#!/bin/bash

set -e
set -x

TAG="$EVG_TRIGGERED_BY_TAG"
if [ -z "$TAG" ]; then
	echo "Cannot regenerate the Augmented SBOM file without a tag"
	exit 1
fi

cat <<EOF >silkbomb.env
SILK_CLIENT_ID=${SILK_CLIENT_ID}
SILK_CLIENT_SECRET=${SILK_CLIENT_SECRET}
EOF
podman run \
	-it --rm \
	--platform linux/amd64 \
	-v ${PWD}:/pwd \
	--env-file silkbomb.env \
	artifactory.corp.mongodb.com/release-tools-container-registry-public-local/silkbomb:1.0 \
	download \
	--silk-asset-group database-tools \
	--sbom-out /pwd/ssdlc/"$TAG".bom.json
