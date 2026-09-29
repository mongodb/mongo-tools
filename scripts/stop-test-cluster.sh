#!/bin/bash

# Stops a cluster started by scripts/start-test-cluster.sh. This is a no-op when the cluster was not
# started, so it is safe to run unconditionally.
#
# Usage:
#   ./scripts/stop-test-cluster.sh [--id=<id>]

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"

CLUSTER_ID="${CLUSTER_ID:-tools-test}"

while [ $# -gt 0 ]; do
    # Accept both "--id value" and "--id=value". Bash's built-in getopts only handles short options,
    # and GNU getopt (which does handle long options) is not available on macOS, so rewrite the
    # separated form to the --id=value form the parser below expects.
    case "$1" in
    --id)
        if [ $# -lt 2 ]; then
            echo "option $1 requires a value" >&2
            exit 1
        fi
        set -- "$1=$2" "${@:3}"
        ;;
    esac

    case "$1" in
    --id=*) CLUSTER_ID="${1#*=}" ;;
    --help | -h)
        sed -n '3,9p' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    *)
        echo "unknown argument: $1" >&2
        exit 1
        ;;
    esac
    shift
done

CLUSTER_DIR="${REPO_ROOT:?}/test-cluster/${CLUSTER_ID:?}"

# This runs in the Evergreen project-wide post block, so it has to be a no-op for the tasks that
# never started a cluster. We check for logs as well as the connection string because a start that
# failed partway through leaves logs but no connection string.
if [ ! -f "${CLUSTER_DIR:?}/connection-string" ] && [ ! -d "${CLUSTER_DIR:?}/logs" ]; then
    echo "no ${CLUSTER_DIR} to stop" >&2
    exit 0
fi

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

"${RUNNER[@]}" stop --id="${CLUSTER_ID:?}"

rm -f "${CLUSTER_DIR:?}/connection-string" "${CLUSTER_DIR:?}/expansions.yml"
