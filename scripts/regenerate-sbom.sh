#!/bin/bash

set -e
set -x

go list -json -mod=mod all |
	jq -r '.Module // empty | "pkg:golang/" + .Path + "@" + .Version // empty' |
	sort -u >purls.txt
go version |
	sed 's|^go version \([^ ]*\) *.*|pkg:golang/std@\1|' >>purls.txt
# The arguments to the silkbomb program start at "update".
podman run \
	-it \
	--rm \
	-v ${PWD}:/pwd \
	artifactory.corp.mongodb.com/release-tools-container-registry-public-local/silkbomb:1.0 \
	update \
	--sbom-in /pwd/cyclonedx.sbom.json \
	--purls /pwd/purls.txt \
	--sbom-out /pwd/cyclonedx.sbom.json
