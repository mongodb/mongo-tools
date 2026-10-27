#!/bin/bash

# Starts a 2-node disaggregated-storage (DSC) replica set for running the tools integration suite
# against, and prints the connection string to export as TOOLS_TESTING_MONGOD.
#
# Everything except mongodb-runner itself comes out of one tarball: the DSC-capable binaries and the
# SLS files (compose file, manifest, and helpers). The runner reads them from the tarball rather than
# from a mongo repo checkout so they can never drift from the Server binaries they were built
# alongside.
#
# This script starts a cluster and then exits, leaving it running. Cluster startup requires starting
# a docker compose project and, on a first run, several minutes of SLS image pulls.
#
# Prerequisites: docker compose v2, python3, `mise install` for mongodb-runner, and the DSC binaries
# already extracted under dsc-cluster/server. The "download mongod and shell" Evergreen function
# fetches them; locally, run scripts/download-mongod-and-shell.sh with MONGO_VERSION=9.1-dsc and
# DISAGG=true first.
#
# Use scripts/stop-dsc-cluster.sh to stop the cluster.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "$0")"

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

# Absolute, not relative: mongodb-runner spawns mongod from its own working directory rather than
# ours, so a relative --binDir fails with "spawn dsc-cluster/server/bin/mongod ENOENT" even though
# the binary is there. Deriving from $0 also lets this run from any working directory.
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"
INSTALL_DIR="${CLUSTER_DIR:?}/server"
LOG_DIR="${CLUSTER_DIR:?}/logs"
CLUSTER_ID="tools-dsc"

if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose v2 is required; see the DSC prerequisites in mongodb-runner's docs" >&2
    exit 1
fi

# We want to actually run the tool rather than just resolving its path: a restored tool cache can
# leave bin/mongodb-runner in place but unable to load its own package (see the comment on
# recreate_npm_bin_symlinks in scripts/install-mise-managed-tools.sh).
if ! "${RUNNER[@]}" --help >/dev/null 2>&1; then
    echo 'mongodb-runner is not installed or cannot load; run "mise install"' >&2
    exit 1
fi

# The DSC binaries and SLS files are downloaded by the "download mongod and shell" Evergreen function
# before this script runs.

mkdir -p "${CLUSTER_DIR:?}" "${LOG_DIR:?}"

# The SLS images live in a private registry, so we log in before compose tries to pull them. The
# login runs under SLS_ECR_AWS_PROFILE, which authenticate-sls-ecr.sh reads directly.
#
# Evergreen does its own login in an earlier step, under an assumed role it does not pass to this
# script. That login is still in effect, since it is recorded in ~/.docker/config.json.
if [ -z "${EVG_WORKDIR:-}" ]; then
    "${SCRIPT_DIR:?}/authenticate-sls-ecr.sh"
fi

# The runner locates and validates the SLS files under the build's buildscripts/modules/atlas
# directory and reads the SLS image tag from the manifest there, so we do not parse the tarball
# ourselves. This ensures that we test using the same Server binary/SLS tag combo that the Server
# tests with.

echo "Starting a DSC replica set (SLS image tag from the build's manifest). The first run pulls all SLS images and can take several minutes." >&2

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
# and the per-mongod logs. The runner writes its diagnostics to stderr and only the JSON result to
# stdout, so capturing stdout keeps it parseable while the debug output stays visible on the
# terminal.
#
# The trailing `--` arguments are passed straight to mongod. They enable the metrics-filtering
# feature flags that a DSC cluster runs with in production, so the integration suite exercises the
# same filtered metrics a real customer would see. The mongosync tool enables the same four flags.
OUTPUT="$("${RUNNER[@]}" start \
    --topology=replset \
    --slsDir="${INSTALL_DIR:?}" \
    --binDir="${INSTALL_DIR:?}/bin" \
    --logDir="${LOG_DIR:?}" \
    --id="${CLUSTER_ID:?}" \
    --json \
    --debug \
    -- \
    --setParameter featureFlagCollStatsMetricsFiltering=true \
    --setParameter featureFlagMetricsFiltering=true \
    --setParameter featureFlagReplSetGetStatusMetricsFiltering=true \
    --setParameter featureFlagServerStatusMetricsFiltering=true)"

# The runner prints a single-line JSON object with the connection string last. The extraction can
# legitimately find nothing when the cluster comes up but the output is malformed, so the `if` here
# is what keeps errexit from swallowing the diagnostic below.
CONNECTION_STRING="$(printf '%s\n' "${OUTPUT:?}" | python3 "${SCRIPT_DIR:?}/parse-connection-string.py" || true)"
if [ -z "$CONNECTION_STRING" ]; then
    echo "the cluster started but printed no connection string; check ${LOG_DIR}" >&2
    exit 1
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

# The expansions.update command in common.yml reads this file. Default to TOOLS_TESTING_MONGOD so the
# single-cluster DSC task is unchanged; the cross-cluster task sets EVG_EXPANSION_VAR to
# TOOLS_TESTING_MONGOD2 so a separately-started 9.0 ASC mongod can own the primary var.
EVG_EXPANSION_VAR="${EVG_EXPANSION_VAR:-TOOLS_TESTING_MONGOD}"
echo "${EVG_EXPANSION_VAR:?}: '${CONNECTION_STRING:?}'" >"${CLUSTER_DIR:?}/expansions.yml"

# Now that the cluster is running properly, we don't want to stop it when the script exits.
trap - EXIT

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
