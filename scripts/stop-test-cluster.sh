#!/bin/bash

# Stops a cluster started by scripts/start-test-cluster.sh.
#
# Usage:
#   ./scripts/stop-test-cluster.sh [--id=<id>]

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"

CLUSTER_ID=tools-test

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
        sed -n '3,6p' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    *)
        echo "unknown argument: $1" >&2
        exit 1
        ;;
    esac
    shift
done

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

"${RUNNER[@]}" stop --id="${CLUSTER_ID:?}"

rm -f "${REPO_ROOT:?}/test-cluster/${CLUSTER_ID:?}/connection-string"
