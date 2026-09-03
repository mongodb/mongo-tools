#!/bin/bash

# Starts a 2-node disaggregated-storage (DSC) replica set for running the tools integration suite
# against, and prints the connection string to export as TOOLS_TESTING_MONGOD.
#
# Everything except mongodb-runner itself comes out of one tarball: the DSC-capable binaries, the
# SLS compose file, and the pinned SLS image tag. The compose file and the image tag are read from
# the tarball rather than from a mongo repo checkout so they can never drift from the Server
# binaries they were built alongside.
#
# This script starts a cluster and then exits, leaving it running. Cluster startup requires starting
# a docker compose project and, on a first run, several minutes of SLS image pulls.
#
# Prerequisites: docker compose v2, `mise install` for mongodb-runner, and read access to the
# private 10gen/mongodb-downloader repo, either through gh or through GITHUB_TOKEN.
#
# Use scripts/stop-dsc-cluster.sh to stop the cluster.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "$0")"
# shellcheck source=scripts/functions.sh
source "${SCRIPT_DIR}/functions.sh"

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

# mise.dsc.toml, which adds mongodb-downloader, is only visible under this MISE_ENV. See the
# comments in that file for more details on why it exists.
export MISE_ENV=dsc
DOWNLOADER=(mise exec github:10gen/mongodb-downloader -- mongodb-downloader)

VERSION_LABEL="9.1-dsc"
# Absolute, not relative: mongodb-runner spawns mongod from its own working directory rather than
# ours, so a relative --binDir fails with "spawn dsc-cluster/server/bin/mongod ENOENT" even though
# the binary is there. Deriving from $0 also lets this run from any working directory.
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"
INSTALL_DIR="${CLUSTER_DIR:?}/server"
LOG_DIR="${CLUSTER_DIR:?}/logs"
CLUSTER_ID="tools-dsc"
ATLAS_DIR="${INSTALL_DIR:?}/buildscripts/modules/atlas"

if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose v2 is required; see the DSC prerequisites in mongodb-runner's docs" >&2
    exit 1
fi

# We use Python to read the SLS image tag from the `manifest.json` file in the Server tarball.
if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to read pinned_sls_commit from the manifest JSON file in the Server tarball" >&2
    exit 1
fi

# We want to actually run the tool rather than just resolving its path: a restored tool cache can
# leave bin/mongodb-runner in place but unable to load its own package (see the comment on
# recreate_npm_bin_symlinks in scripts/install-mise-managed-tools.sh).
if ! "${RUNNER[@]}" --help >/dev/null 2>&1; then
    echo 'mongodb-runner is not installed or cannot load; run "mise install"' >&2
    exit 1
fi

# Installed here rather than in a separate step because it comes from a private repo: it cannot be
# part of the plain "mise install" that everything else in mise.toml goes through. The token comes
# from GITHUB_TOKEN in Evergreen and from "gh auth token" locally (see mise.dsc.toml). We only retry
# twice because the likely failure is a token that cannot read the repo, which is not transient.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install github:10gen/mongodb-downloader

mkdir -p "${CLUSTER_DIR:?}" "${LOG_DIR:?}"

# The SLS images live in a private registry, so we log in before compose tries to pull them. The
# login runs under SLS_ECR_AWS_PROFILE, which authenticate-sls-ecr.sh reads directly, so the
# download below still uses whatever AWS_PROFILE the caller set.
#
# Evergreen does its own login, in a step before it assumes the role that reads Server builds from
# S3, because only one set of AWS_* variables can be in the environment at a time. Repeating it here
# would run under the S3 role, which cannot mint an ECR token. The docker login from that earlier
# step is still in effect, since it is recorded in ~/.docker/config.json.
if [ -z "${EVG_WORKDIR:-}" ]; then
    "${SCRIPT_DIR:?}/authenticate-sls-ecr.sh"
fi

"${DOWNLOADER[@]}" download \
    --config "${REPO_ROOT:?}/etc/mongodb-downloader-config.yaml" \
    --output-dir "${REPO_ROOT:?}/etc/" \
    --server-version "${VERSION_LABEL:?}" \
    --to "${INSTALL_DIR:?}"

COMPOSE_FILE="${ATLAS_DIR:?}/sls-multicell-docker-compose.yml"
MANIFEST="${ATLAS_DIR:?}/manifest.json"

for f in "${COMPOSE_FILE:?}" "${MANIFEST:?}" "${ATLAS_DIR:?}/slsbackup.proto" "${ATLAS_DIR:?}/flags-state.json"; do
    if [ ! -f "$f" ]; then
        echo "missing ${f}: the downloaded tarball is not a disagg build" >&2
        exit 1
    fi
done

# We read the SLS image tag from the manifest bundled with the Server binaries. This ensures that we
# test using the same Server binary/SLS tag combo that the Server tests with.
SLS_IMAGE_TAG="$(python3 -c "import json; print(json.load(open('${MANIFEST:?}'))['pinned_sls_commit'])")"

echo "Starting a DSC replica set (SLS image tag ${SLS_IMAGE_TAG:?}). The first run pulls all SLS images and can take several minutes." >&2

cleanup_started_cluster() {
    echo "start failed after the cluster came up; tearing it down" >&2
    "${SCRIPT_DIR:?}/stop-dsc-cluster.sh" || echo "teardown also failed; run scripts/stop-dsc-cluster.sh by hand" >&2
}

# stop-dsc-cluster.sh checks for the marker file before it tears anything down. We create it and
# install the cleanup trap before starting the cluster so that a failed `mongodb-runner start`
# (which errexit/pipefail would otherwise let end the script) still tears the cluster down.
touch "${CLUSTER_DIR:?}/connection-string"
trap cleanup_started_cluster EXIT

# Without --debug a DSC startup failure is very hard to diagnose: details are in the compose output
# and the per-mongod logs. The runner writes its diagnostics to stderr and only the connection
# string to stdout, so capturing stdout keeps it parseable; the tee is there to keep those
# diagnostics visible on the terminal as they happen.
OUTPUT="$("${RUNNER[@]}" start \
    --topology=replset \
    --slsCompose="${COMPOSE_FILE:?}" \
    --slsImageTag="${SLS_IMAGE_TAG:?}" \
    --binDir="${INSTALL_DIR:?}/bin" \
    --logDir="${LOG_DIR:?}" \
    --id="${CLUSTER_ID:?}" \
    --debug | tee /dev/stderr)"

# The runner prints the cluster URI last, after the per-node startup output. The grep can legitimately
# fail when the cluster comes up but the URI is missing from the output, so the `if !` here is what
# keeps errexit from swallowing the diagnostic below.
if ! CONNECTION_STRING="$(echo "${OUTPUT:?}" | grep -oE 'mongodb://[^ ]+' | tail -1)"; then
    echo "the cluster started but printed no mongodb:// connection string; check ${LOG_DIR}" >&2
    exit 1
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

# Evergreen's expansions.update reads a YAML file. This is written unconditionally so that a local
# run produces the same artifacts as a CI run.
echo "TOOLS_TESTING_MONGOD: '${CONNECTION_STRING:?}'" >"${CLUSTER_DIR:?}/expansions.yml"

# Now that the cluster is running properly, we don't want to stop it when the script exits.
trap - EXIT

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
