#!/bin/bash

# Authenticates docker to the ECR registry hosting the SLS container images, which back a
# disaggregated-storage (DSC) cluster started by scripts/start-dsc-cluster.sh. See TOOLS-4100.
#
# Runs both locally and in Evergreen. It needs no EVG_WORKDIR handling because it uses only the aws
# and docker CLIs from the distro, not anything mise installs.
#
# The credentials that can pull SLS images are NOT the same ones that can read the Server binary
# tarballs from S3, so this takes its profile from SLS_ECR_AWS_PROFILE rather than sharing
# AWS_PROFILE with the rest of the toolchain. Using one profile for both makes whichever half runs
# second fail. SLS_ECR_AWS_PROFILE falls back to AWS_PROFILE when unset, so running this script on
# its own still works.
#
# Evergreen has no profiles at all: ec2.assume_role hands back AWS_ACCESS_KEY_ID and friends in the
# environment, which the aws CLI picks up on its own. So the task leaves SLS_ECR_AWS_PROFILE unset
# and this script sets no AWS_PROFILE, which is why the ordering in the DSC task matters -- a second
# ec2.assume_role overwrites those variables, so the ECR login has to happen before the S3 role is
# assumed.
#
# Note that a successful login here does NOT prove you can pull: minting a token only needs
# ecr:GetAuthorizationToken in your own account, while pulling needs ecr:BatchGetImage granted by a
# resource-based policy on the repositories in account 664315256653. Login succeeds with profiles
# that cannot pull, so verify with `docker manifest inspect` if a pull later fails.

set -o errexit
set -o nounset
set -o pipefail

# These are literals rather than inputs, so they carry no ":?" guard -- a guard on a value assigned
# one line above can never fire, and writing one implies these come from the environment.
REGISTRY_ID="664315256653"
ECR="${REGISTRY_ID}.dkr.ecr.us-east-1.amazonaws.com"
REGION="us-east-1"

if [ -n "${SLS_ECR_AWS_PROFILE:-}" ]; then
    export AWS_PROFILE="${SLS_ECR_AWS_PROFILE:?}"
fi

# This must be get-authorization-token with --registry-ids, not the more familiar
# get-login-password. get-login-password issues a token scoped to the caller's OWN registry. Locally
# that is the wrong account -- a developer's profile lives outside 664315256653 -- so docker login
# rejects the token with "status: 400 Bad Request". Verified by hand: the --registry-ids form below
# succeeds where get-login-password does not.
#
# In Evergreen the assumed role is inside 664315256653, so get-login-password would work there. We
# still use one form for both, since a command that only works under CI is a command nobody can
# debug from their laptop.
set -o xtrace
aws ecr get-authorization-token \
    --region "${REGION}" \
    --registry-ids "${REGISTRY_ID}" \
    --query 'authorizationData[0].authorizationToken' \
    --output text |
    base64 -d | cut -d: -f2- |
    docker login --username AWS --password-stdin "${ECR}"
