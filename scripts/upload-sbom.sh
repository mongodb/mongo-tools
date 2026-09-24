#!/bin/bash

# Uploads the current SBOM to the Vulnerability Management Platform via `silkbomb upload`.
# Generates a fresh SBOM first (see scripts/generate-sbom.sh) rather than reusing a
# previously-generated file: there's no developer-maintained, checked-in SBOM to reuse (see
# TOOLS-3768).
#
# Requires SBOM_BRANCH to be set: the branch name (mainline commits) or release tag (release
# builds) this upload should be attributed to.
#
# Credentials: expects a silkbomb.env file at the repo root (written by the "write silkbomb
# environment file" Evergreen function) containing AWS credentials for a role that can resolve
# silkbomb's required API credentials from Secrets Manager itself -- silkbomb does this
# automatically when given AWS credentials and no token is set directly.
#
# Writes the generated SBOM to sbom.json in the repo root (gitignored) rather than a temp dir, so
# the "upload-sbom" Evergreen task can attach it as an artifact via s3.put after this script runs.

set -e
set -o pipefail
set -x

if [ -z "${SBOM_BRANCH:-}" ]; then
    echo "SBOM_BRANCH must be set" >&2
    exit 1
fi

./scripts/generate-sbom.sh --output sbom.json

RUNTIME="$(./scripts/container-runtime.sh)"
"$RUNTIME" run \
    --rm \
    --platform linux/amd64 \
    -v "$(pwd)":/workdir \
    --env-file silkbomb.env \
    901841024863.dkr.ecr.us-east-1.amazonaws.com/release-infrastructure/silkbomb:2.0 \
    upload \
    --sbom-in /workdir/sbom.json \
    --repo mongodb/mongo-tools \
    --branch "$SBOM_BRANCH"
