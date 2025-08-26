#!/bin/bash

set -e
set -o pipefail
set -x

rm -f purls.txt

BINARY_DIRS="$( go run release/release.go print-binary-paths )"
OS_ARCH_COMBOS="$( go run release/release.go print-os-arch-combos )"

# This set of piped commands generates a file that contains each dependency as a purl
# (https://github.com/package-url/purl-spec), one per line. This is used as input for the `silkbomb`
# tool to generate an SBOM. We do this for each OS/architecture combination we support to make sure
# this is the superset of all our dependencies.
# We skip the mongo-tools module in the jq query so that it's not listed as a
# dependency of itself (.Module.Main is set to true for mongo-tools)
#
# shellcheck disable=SC2086 # we intentionally don't quote `$OS_ARCH_COMBOS` so we split on the
# whitespace.
for c in $OS_ARCH_COMBOS; do
    os="$(echo $c | cut -f1 -d/)"
    arch="$(echo $c | cut -f2 -d/)"
    # shellcheck disable=SC2086 # we don't want to quote `$BINARY_DIRS` for the same reason.
    GOOS="$os" GOARCH="$arch" go list -json -mod=mod -deps $BINARY_DIRS |
        jq -r '.Module // empty | select((.Main // false) == false) | "pkg:golang/" + .Path + "@" + .Version // empty' >> \
            purls.txt
done

sort -u -o purls.txt purls.txt

if [ ! -s purls.txt ]; then
    echo 'The purls.txt file generated from the "go list" output is empty!'
    exit 1
fi

go version |
    sed 's|^go version \([^ ]*\) *.*|pkg:golang/std@\1|' >>purls.txt

# The arguments to the silkbomb program start at "update".
#
# shellcheck disable=SC2068 # we don't want to quote `$@`.
podman run \
    --rm \
    --platform linux/amd64 \
    -v "${PWD}":/pwd \
    artifactory.corp.mongodb.com/release-tools-container-registry-public-local/silkbomb:2.0 \
    update \
    --sbom-in /pwd/cyclonedx.sbom.json \
    --purls /pwd/purls.txt \
    --sbom-out /pwd/cyclonedx.sbom.json \
    $@
