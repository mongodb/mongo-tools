#!/bin/bash

# Authenticates podman to the DevProd Platforms ECR registry (which hosts silkbomb and garasign).
#
# Called two ways in Evergreen:
#   - Directly, right after ec2.assume_role, by the "authenticate with devprod platforms ecr"
#     function (used by the "sign" and "check-sbom-lite" tasks) - AWS_ACCESS_KEY_ID etc. are
#     already set from that assumed role, so this logs in with them directly.
#   - Indirectly, from etc/regenerate-sbom-lite.sh / etc/regenerate-augmented-sbom.sh. When those
#     run as part of "check-sbom-lite", the task already authenticated via the path above, so this
#     is a no-op (EVG_WORKDIR is set, but AWS_ACCESS_KEY_ID isn't, since it wasn't requested for
#     this specific subprocess).
#
# Locally (neither of the above), uses a named AWS SSO profile instead. Requires membership in the
# devprod-platforms-ecr-users Okta group and an AWS SSO profile named "ECRScopedAccess-901841024863"
# for the account; see https://docs.devprod.prod.corp.mongodb.com/devprod-platforms-ecr for
# instructions on how to set this up.

set -o errexit
set -o pipefail

UNAME="$(uname -s)"

# We never need to run podman on macOS in CI, and it's not installed there, so we can't even if we
# wanted to.
if [[ -n ${EVG_WORKDIR:-} && ${UNAME:?} =~ Darwin ]]; then
    exit 0
fi

ECR="901841024863.dkr.ecr.us-east-1.amazonaws.com"
REGION="us-east-1"

if [ -n "${AWS_ACCESS_KEY_ID:-}" ]; then
    set -o xtrace
    aws ecr get-login-password --region "${REGION:?}" | podman login --username AWS --password-stdin "${ECR:?}"
elif [ -n "${EVG_WORKDIR:-}" ]; then
    exit 0
else
    set -o xtrace
    PROFILE="ECRScopedAccess-901841024863"
    aws ecr get-login-password --region "${REGION:?}" --profile "${PROFILE:?}" | podman login --username AWS --password-stdin "${ECR:?}"
fi
