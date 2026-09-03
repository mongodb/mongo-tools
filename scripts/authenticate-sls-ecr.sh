#!/bin/bash

# Authenticates docker to the ECR registry hosting the SLS container images.
#
# This runs both locally and in Evergreen.
#
# The credentials that can pull SLS images are not the same ones that can read the Server binary
# tarballs from S3, so this takes its profile from `SLS_ECR_AWS_PROFILE` rather than sharing
# `AWS_PROFILE` with the rest of the toolchain. If we tried to use one profile for both, then either
# the Docker or S3 interactions would fail. `SLS_ECR_AWS_PROFILE` falls back to `AWS_PROFILE` when
# unset, so running this script on its own still works.
#
# Evergreen does not set any profile env vars. The `ec2.assume_role` command sets
# `AWS_ACCESS_KEY_ID` and related env vars, which the aws CLI picks up on its own.
#
# Note that a successful login here does not prove you can pull: minting a token only needs
# `ecr:GetAuthorizationToken` in your own account, while pulling needs `ecr:BatchGetImage` granted
# by a resource-based policy on the repositories in account 664315256653. Login succeeds with a
# profile that cannot pull, so verify with `docker manifest inspect` if a pull later fails.

set -o errexit
set -o nounset
set -o pipefail

REGISTRY_ID="664315256653"
ECR="${REGISTRY_ID}.dkr.ecr.us-east-1.amazonaws.com"
REGION="us-east-1"

if [ -n "${SLS_ECR_AWS_PROFILE:-}" ]; then
    export AWS_PROFILE="${SLS_ECR_AWS_PROFILE:?}"
fi

# This must be get-authorization-token with --registry-ids, not the more familiar
# get-login-password. get-login-password issues a token scoped to the caller's _own_
# registry. Locally that is the wrong account - a developer's profile lives outside 664315256653 -
# so docker login rejects the token with "status: 400 Bad Request".
#
# In Evergreen the assumed role is inside `664315256653`, so get-login-password would work there,
# but it's simpler to use the same command for both cases.
set -o xtrace
aws ecr get-authorization-token \
    --region "${REGION}" \
    --registry-ids "${REGISTRY_ID}" \
    --query 'authorizationData[0].authorizationToken' \
    --output text |
    base64 -d | cut -d: -f2- |
    docker login --username AWS --password-stdin "${ECR}"
