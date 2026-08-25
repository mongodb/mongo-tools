#!/bin/bash

# Tears down a DSC cluster started by scripts/start-dsc-cluster.sh, including the docker compose
# project providing the SLS storage backend. See TOOLS-4100.
#
# This is a no-op when no cluster was started here, because Evergreen's post block runs for every
# task in the project, not just the DSC one.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "$0")"
CLUSTER_ID="tools-dsc"
# Derived the same way start-dsc-cluster.sh derives it, so this works from any working directory.
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"
CLUSTER_DIR="${REPO_ROOT:?}/dsc-cluster"

# Gate on the connection string that start-dsc-cluster.sh writes, rather than on whether the runner
# is installed: every task in the project installs mise-managed tools, but only the DSC one starts a
# cluster.
if [ ! -f "${CLUSTER_DIR:?}/connection-string" ]; then
    echo "no ${CLUSTER_DIR}/connection-string, so there is no DSC cluster to stop" >&2
    exit 0
fi

if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

RUNNER=(mise exec node npm:mongodb-runner -- mongodb-runner)

# Archive the logs before tearing the cluster down, and do it here rather than with an s3.put file
# filter, because this runs in Evergreen's project-wide post block. A filter that matches nothing
# would have to rely on s3.put honoring "optional" for multi-file puts, which nothing in this repo
# demonstrates; a single optional local_file is a form this repo already proves. Failing to archive
# must not stop the teardown, which is the part that actually matters.
if [ -d "${CLUSTER_DIR:?}/logs" ]; then
    tar -czf "${CLUSTER_DIR:?}/logs.tgz" -C "${CLUSTER_DIR:?}" logs ||
        echo "could not archive the cluster logs; tearing down anyway" >&2
fi

"${RUNNER[@]}" stop --id="${CLUSTER_ID:?}"
rm -f "${CLUSTER_DIR:?}/connection-string" "${CLUSTER_DIR:?}/expansions.yml"
