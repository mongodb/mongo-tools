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
# Prerequisites: docker compose v2, "mise install" for mongodb-runner, and read access to the private
# 10gen/mongodb-downloader repo, either through gh or through GITHUB_TOKEN.
#
# Tear down with scripts/stop-dsc-cluster.sh.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "$0")"
# shellcheck source=scripts/functions.sh
source "${SCRIPT_DIR}/functions.sh"

# Optional, because whether one profile can do both jobs depends on how your AWS access is set up.
# Two different accounts are involved: the Server tarball lives in an S3 bucket one account can read,
# and the SLS images live in ECR account 664315256653, which a different account may be the one
# permitted to pull from. Set SLS_ECR_AWS_PROFILE to run just the ECR login under its own profile,
# leaving AWS_PROFILE to the downloader; leave it unset if a single profile covers both.
#
# Note that a profile can log in to the registry successfully and still be unable to pull, because
# minting a token only needs access in the caller's own account.

# mise.toml manages both node and mongodb-runner, so the runner is invoked through "mise exec"
# rather than off PATH. Run "mise install" before this script. In Evergreen, mise itself is under
# ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

# Every tool mise exec needs has to be named explicitly. Without that list mise tries to install
# every tool in mise.toml, which fails on some CI platforms.
RUNNER=(mise exec node npm:mongodb-runner -- mongodb-runner)

# mise.dsc.toml, which adds mongodb-downloader, is only visible under this MISE_ENV. See the comment
# at the top of that file for why the downloader is not in mise.toml with everything else.
export MISE_ENV=dsc
DOWNLOADER=(mise exec github:10gen/mongodb-downloader -- mongodb-downloader)

VERSION_LABEL="9.0-dsc"
# Absolute, not relative: mongodb-runner spawns mongod from its own working directory rather than
# ours, so a relative --binDir fails with "spawn dsc-cluster/server/bin/mongod ENOENT" even though
# the binary is right there. Deriving from $0 also lets this run from any working directory.
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"
INSTALL_DIR="${CLUSTER_DIR:?}/server"
LOG_DIR="${CLUSTER_DIR:?}/logs"
# A fixed id rather than one parsed out of the runner's stdout: --id is an input flag, so both
# start and stop can simply name the same cluster.
CLUSTER_ID="tools-dsc"
ATLAS_DIR="${INSTALL_DIR:?}/buildscripts/modules/atlas"

# Fail on missing prerequisites up front with a message that names the fix, rather than partway
# through a compose startup where the cause is buried in container logs.
if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose v2 is required; see the DSC prerequisites in mongodb-runner's docs" >&2
    exit 1
fi

# Actually running the tool, rather than just resolving its path, is deliberate: a cached npm
# install can leave bin/mongodb-runner in place but unable to load its own package (see the
# cache-hit comment in scripts/install-mise-managed-tools.sh), and a path check passes on that.
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

# LOG_DIR is created here rather than left to the runner: if it did not create it, the only
# symptom would be an empty log upload in CI, and empty is indistinguishable from "no DSC cluster".
mkdir -p "${CLUSTER_DIR:?}" "${LOG_DIR:?}"

# The SLS images live in a private registry, so log in before compose tries to pull them. The login
# runs under SLS_ECR_AWS_PROFILE, which authenticate-sls-ecr.sh reads directly, so the download
# below still uses whatever AWS_PROFILE the caller set.
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
    "${SCRIPT_DIR:?}/stop-dsc-cluster.sh" || echo "teardown also failed; run scripts/stop-dsc-cluster.sh by hand" >&2
}

# --debug because a DSC startup failure is otherwise very hard to diagnose: the useful detail is in
# the compose output and the per-mongod logs. The runner writes its diagnostics to stderr and only
# the connection string to stdout, so capturing stdout keeps it parseable; the tee is there purely
# to keep those diagnostics visible on the terminal as they happen.
OUTPUT="$("${RUNNER[@]}" start \
    --topology=replset \
    --slsCompose="${COMPOSE_FILE:?}" \
    --slsImageTag="${SLS_IMAGE_TAG:?}" \
    --binDir="${INSTALL_DIR:?}/bin" \
    --logDir="${LOG_DIR:?}" \
    --id="${CLUSTER_ID:?}" \
    --debug | tee /dev/stderr)"

# Create the file that stop-dsc-cluster.sh gates on before anything else can fail, and fill it in
# below. A cluster exists from here on, so a later failure has to be able to tear it down.
touch "${CLUSTER_DIR:?}/connection-string"
trap cleanup_started_cluster EXIT

# The runner prints the cluster URI last, after the per-node startup chatter, so take the final
# match rather than the first.
CONNECTION_STRING="$(echo "${OUTPUT:?}" | grep -oE 'mongodb://[^ ]+' | tail -1)"
if [ -z "${CONNECTION_STRING}" ]; then
    echo "the cluster started but printed no mongodb:// connection string; check ${LOG_DIR}" >&2
    exit 1
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

# Evergreen's expansions.update reads a YAML file, and it cannot read a variable we exported,
# because every command in a task runs in its own shell. This is written unconditionally so that a
# local run produces the same artifacts as a CI run.
echo "TOOLS_TESTING_MONGOD: '${CONNECTION_STRING:?}'" >"${CLUSTER_DIR:?}/expansions.yml"

# The cluster is up and usable, so stop treating exit as a failure -- leaving it running is the
# whole point of this script.
trap - EXIT

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
