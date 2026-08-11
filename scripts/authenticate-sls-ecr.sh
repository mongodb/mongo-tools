#!/bin/bash

# Authenticates docker to the ECR registry hosting the SLS container images, which back a
# disaggregated-storage (DSC) cluster started by scripts/start-dsc-cluster.sh. See TOOLS-4100.
#
# Local use only for now. There is no Evergreen path yet, so this script has no EVG_WORKDIR
# handling.
#
# The credentials that can pull SLS images are NOT the same ones that can read the Server binary
# tarballs from S3, so this takes its profile from SLS_ECR_AWS_PROFILE rather than sharing
# AWS_PROFILE with the rest of the toolchain. Using one profile for both makes whichever half runs
# second fail. SLS_ECR_AWS_PROFILE falls back to AWS_PROFILE when unset, so running this script on
# its own still works.
#
# Note that a successful login here does NOT prove you can pull: minting a token only needs
# ecr:GetAuthorizationToken in your own account, while pulling needs ecr:BatchGetImage granted by a
# resource-based policy on the repositories in account 664315256653. Login succeeds with profiles
# that cannot pull, so verify with `docker manifest inspect` if a pull later fails.

set -o errexit
set -o pipefail

REGISTRY_ID="664315256653"
ECR="${REGISTRY_ID:?}.dkr.ecr.us-east-1.amazonaws.com"
REGION="us-east-1"

if [ -n "${SLS_ECR_AWS_PROFILE:-}" ]; then
    export AWS_PROFILE="${SLS_ECR_AWS_PROFILE:?}"
fi

# This must be get-authorization-token with --registry-ids, not the more familiar
# get-login-password. get-login-password issues a token scoped to the caller's OWN registry, and
# the SLS images live in a different account, so docker login rejects that token with
# "status: 400 Bad Request". Verified by hand: the --registry-ids form below succeeds where
# get-login-password does not.
set -o xtrace
aws ecr get-authorization-token \
    --region "${REGION:?}" \
    --registry-ids "${REGISTRY_ID:?}" \
    --query 'authorizationData[0].authorizationToken' \
    --output text |
    base64 -d | cut -d: -f2- |
    docker login --username AWS --password-stdin "${ECR:?}"
