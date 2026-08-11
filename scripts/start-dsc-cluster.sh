#!/bin/bash

# Starts a 2-node disaggregated-storage (DSC) replica set for running the tools integration suite
# against, and prints the connection string to export as TOOLS_TESTING_MONGOD. See TOOLS-4100.
#
# Everything except mongodb-runner itself comes out of one tarball: the DSC-capable binaries, the
# SLS compose file, and the pinned SLS image tag. That is deliberate. The compose file and the
# image tag are read from the tarball rather than from a mongo repo checkout so they can never
# drift from the Server binaries they were built alongside.
#
# This script starts a cluster and then exits, leaving it running. It does not run tests. Cluster
# startup pays for a docker compose project and, on a first run, several minutes of SLS image pulls,
# and that cost should not be re-paid on every test iteration while triaging failures.
#
# Tear down with scripts/stop-dsc-cluster.sh.

set -o errexit
set -o nounset
set -o pipefail

: "${MONGODB_DOWNLOADER:?path to the built mongodb-downloader binary}"
: "${DEVTOOLS_SHARED:?path to a checkout of devtools-shared PR 822 (DSC support)}"

# Optional, because whether one profile can do both jobs depends on how your AWS access is set up.
# Two different accounts are involved: the Server tarball lives in an S3 bucket one account can read,
# and the SLS images live in ECR account 664315256653, which a different account may be the one
# permitted to pull from. Set SLS_ECR_AWS_PROFILE to run just the ECR login under its own profile,
# leaving AWS_PROFILE to the downloader; leave it unset if a single profile covers both.
#
# Note that a profile can log in to the registry successfully and still be unable to pull, because
# minting a token only needs access in the caller's own account.

VERSION_LABEL="9.0-dsc"
# Absolute, not relative: mongodb-runner spawns mongod from its own working directory rather than
# ours, so a relative --binDir fails with "spawn dsc-cluster/server/bin/mongod ENOENT" even though
# the binary is right there. Deriving from $0 also lets this run from any working directory.
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"
INSTALL_DIR="${CLUSTER_DIR:?}/server"
LOG_DIR="${CLUSTER_DIR:?}/logs"
# A fixed id rather than one parsed out of the runner's stdout: --id is an input flag, so both
# start and stop can simply name the same cluster.
CLUSTER_ID="tools-dsc"
RUNNER="${DEVTOOLS_SHARED:?}/packages/mongodb-runner/bin/runner.js"
ATLAS_DIR="${INSTALL_DIR:?}/buildscripts/modules/atlas"

# Fail on missing prerequisites up front with a message that names the fix, rather than partway
# through a compose startup where the cause is buried in container logs.
if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose v2 is required; see the DSC prerequisites in mongodb-runner's docs" >&2
    exit 1
fi

if [ ! -f "${RUNNER:?}" ]; then
    echo "no compiled mongodb-runner at ${RUNNER}; run 'npm install' at the checkout root and 'npm run compile' in packages/mongodb-runner" >&2
    exit 1
fi

mkdir -p "${CLUSTER_DIR:?}"

# The SLS images live in a private registry, so log in before compose tries to pull them. The login
# runs under SLS_ECR_AWS_PROFILE, which authenticate-sls-ecr.sh reads directly, so the download
# below still uses whatever AWS_PROFILE the caller set.
"$(dirname "$0")/authenticate-sls-ecr.sh"

"${MONGODB_DOWNLOADER:?}" download \
    --config "${REPO_ROOT:?}/etc/mongodb-downloader-config.yaml" \
    --output-dir "${REPO_ROOT:?}/etc/" \
    --server-version "${VERSION_LABEL:?}" \
    --to "${INSTALL_DIR:?}"

# The disagg tarball is flat: bin/mongo* and buildscripts/modules/atlas sit directly under the
# extraction directory. The compose file resolves slsbackup.proto and flags-state.json relative to
# itself, so it must be used from where it was extracted.
COMPOSE_FILE="${ATLAS_DIR:?}/sls-multicell-docker-compose.yml"
MANIFEST="${ATLAS_DIR:?}/manifest.json"

for f in "${COMPOSE_FILE:?}" "${MANIFEST:?}" "${ATLAS_DIR:?}/slsbackup.proto" "${ATLAS_DIR:?}/flags-state.json"; do
    if [ ! -f "$f" ]; then
        echo "missing ${f}: the downloaded tarball is not a disagg build" >&2
        exit 1
    fi
done

# The SLS image tag must be the pinned commit from the same Server commit as the binaries, which is
# exactly what the tarball's own manifest records. Read it rather than hardcoding it, so the tag can
# never drift from the binaries it was built alongside.
if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to read pinned_sls_commit from ${MANIFEST}" >&2
    exit 1
fi

SLS_IMAGE_TAG="$(python3 -c "import json; print(json.load(open('${MANIFEST:?}'))['pinned_sls_commit'])")"

echo "Starting a DSC replica set (SLS image tag ${SLS_IMAGE_TAG:?}). The first run pulls all SLS images and can take several minutes." >&2

# mongodb-runner tears down its own compose project when startup fails partway through, but once
# start SUCCEEDS the cluster is ours to clean up. Anything that fails after this point would
# otherwise strand a live 2-node cluster and its compose project with no hint about how to get rid
# of them, so from here on a non-zero exit tears the cluster down first.
cleanup_started_cluster() {
    echo "start failed after the cluster came up; tearing it down" >&2
    "$(dirname "$0")/stop-dsc-cluster.sh" || echo "teardown also failed; run scripts/stop-dsc-cluster.sh by hand" >&2
}

# --debug because a DSC startup failure is otherwise very hard to diagnose: the useful detail is in
# the compose output and the per-mongod logs. The runner writes its diagnostics to stderr and only
# the connection string to stdout, so capturing stdout keeps it parseable; the tee is there purely
# to keep those diagnostics visible on the terminal as they happen.
OUTPUT="$(node "${RUNNER:?}" start \
    --topology=replset \
    --slsCompose="${COMPOSE_FILE:?}" \
    --slsImageTag="${SLS_IMAGE_TAG:?}" \
    --binDir="${INSTALL_DIR:?}/bin" \
    --logDir="${LOG_DIR:?}" \
    --id="${CLUSTER_ID:?}" \
    --debug | tee /dev/stderr)"

trap cleanup_started_cluster EXIT

# The runner prints the cluster URI last, after the per-node startup chatter, so take the final
# match rather than the first.
CONNECTION_STRING="$(echo "${OUTPUT:?}" | grep -oE 'mongodb://[^ ]+' | tail -1)"
if [ -z "${CONNECTION_STRING}" ]; then
    echo "the cluster started but printed no mongodb:// connection string; check ${LOG_DIR}" >&2
    exit 1
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

# The cluster is up and usable, so stop treating exit as a failure -- leaving it running is the
# whole point of this script.
trap - EXIT

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
