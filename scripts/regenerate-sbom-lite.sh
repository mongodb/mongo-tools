#!/bin/bash

set -e
set -o pipefail
set -x

# In CI, authentication with the DevProd Platforms ECR registry happens once
# at the Evergreen task level before this script runs. Locally, we log in
# here using a named AWS SSO profile.
if [ -z "${EVG_TASK_ID:-}" ]; then
    profile="${DEVPROD_PLATFORMS_ECR_AWS_PROFILE:-ECRScopedAccess-901841024863}"
    aws ecr get-login-password --region us-east-1 --profile "$profile" |
        podman login --username AWS --password-stdin 901841024863.dkr.ecr.us-east-1.amazonaws.com
fi

rm -f purls.txt

BINARY_DIRS="$(mise exec go -- go run release/release.go print-binary-paths)"
OS_ARCH_COMBOS="$(mise exec go -- go run release/release.go print-os-arch-combos)"

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
    GOOS="$os" GOARCH="$arch" mise exec go -- go list -json -mod=mod -deps $BINARY_DIRS |
        jq -r '.Module // empty | select((.Main // false) == false) | "pkg:golang/" + .Path + "@" + .Version // empty' >> \
            purls.txt
done

sort -u -o purls.txt purls.txt

if [ ! -s purls.txt ]; then
    echo 'The purls.txt file generated from the "go list" output is empty!'
    exit 1
fi

mise exec go -- go version |
    sed 's|^go version \([^ ]*\) *.*|pkg:golang/std@\1|' >>purls.txt

# The arguments to the silkbomb program start at "update".
#
# shellcheck disable=SC2068 # we don't want to quote `$@`.
podman run \
    --rm \
    --platform linux/amd64 \
    -v "${PWD}":/pwd \
    901841024863.dkr.ecr.us-east-1.amazonaws.com/release-infrastructure/silkbomb:2.0 \
    update \
    --sbom-in /pwd/cyclonedx.sbom.json \
    --purls /pwd/purls.txt \
    --sbom-out /pwd/cyclonedx.sbom.json \
    $@
