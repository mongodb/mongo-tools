#!/bin/bash

set -e
set -x

# This set of piped commands generates a file that contains each dependency as
# a purl (https://github.com/package-url/purl-spec), one per line. This is
# used as input for the `silkbomb` tool to generate an SBOM.
go list -json -mod=mod all |
    jq -r '.Module // empty | "pkg:golang/" + .Path + "@" + .Version // empty' |
    sort -u >purls.txt
go version |
    sed 's|^go version \([^ ]*\) *.*|pkg:golang/std@\1|' >>purls.txt
# The arguments to the silkbomb program start at "update".
#
# shellcheck disable=SC2068 # we don't want to quote `$@`
podman run \
    -it \
    --rm \
    -v "${PWD}":/pwd \
    artifactory.corp.mongodb.com/release-tools-container-registry-public-local/silkbomb:1.0 \
    update \
    --sbom-in /pwd/cyclonedx.sbom.json \
    --purls /pwd/purls.txt \
    --sbom-out /pwd/cyclonedx.sbom.json \
    $@
