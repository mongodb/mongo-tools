#!/bin/bash

# Authenticates docker to the ECR registry hosting the SLS container images, which back a
# disaggregated-storage (DSC) cluster started by scripts/start-dsc-cluster.sh. See TOOLS-4100.
#
# Local use only for now. There is no Evergreen path yet, so this script has no EVG_WORKDIR
# handling. Set AWS_PROFILE if your SLS-capable credentials live in a named profile.

set -o errexit
set -o pipefail

REGISTRY_ID="664315256653"
ECR="${REGISTRY_ID:?}.dkr.ecr.us-east-1.amazonaws.com"
REGION="us-east-1"

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
