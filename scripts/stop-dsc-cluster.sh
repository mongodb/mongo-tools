#!/bin/bash

# Tears down a DSC cluster started by scripts/start-dsc-cluster.sh, including the docker compose
# project providing the SLS storage backend.
#
# This is a no-op when the cluster was not started, because we run this in the Evergreen `post`
# block for every task in the project.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "$0")"
CLUSTER_ID="tools-dsc"
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"

# We continue even if there's just a log directory, because a start that fails partway through leaves logs but no
# connection string.
if [ ! -f "${CLUSTER_DIR:?}/connection-string" ] && [ ! -d "${CLUSTER_DIR:?}/logs" ]; then
    echo "no ${CLUSTER_DIR}/connection-string and no ${CLUSTER_DIR}/logs, so there is no DSC cluster to stop" >&2
    exit 0
fi

if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

# Archiving happens after the teardown, so that anything the runner writes into logDir during
# teardown is included.
#
# The teardown's exit status is held rather than allowed to end the script, so that a failed teardown
# still produces an archive.
#
# The teardown is skipped when there is no connection string, because the runner tears its own
# compose project down when start fails.
STOP_STATUS=0
if [ -f "${CLUSTER_DIR:?}/connection-string" ]; then
    "${RUNNER[@]}" stop --id="${CLUSTER_ID:?}" || STOP_STATUS=$?
fi

# Archived here rather than with an `s3.put` because this runs in Evergreen's project-wide post
# block. A filter that matches nothing would require `s3.put` to allow `optional` for multi-file
# puts, which it does not.
if [ -d "${CLUSTER_DIR:?}/logs" ]; then
    tar -czf "${CLUSTER_DIR:?}/logs.tgz" -C "${CLUSTER_DIR:?}" logs ||
        echo "could not archive the cluster logs" >&2
fi

rm -f "${CLUSTER_DIR:?}/connection-string" "${CLUSTER_DIR:?}/expansions.yml"

exit "${STOP_STATUS}"
