#!/bin/bash

# Stops a cluster started by scripts/start-test-cluster.sh, including a DSC cluster started with
# --dsc. This is a no-op when the cluster was not started, so it is safe to run unconditionally.
#
# The logic lives in stop-test-cluster.py. This wrapper keeps Evergreen's `binary: bash` invocation
# (Windows cannot execute a shebang script as a native binary) and inherits the environment
# ci-env.sh sets up, notably mise on PATH.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")

# This runs in Evergreen's project-wide post block on every host, including ones that may not have a
# usable python3, so exit before needing it when there is plainly no cluster to stop. This mirrors
# the argument handling and cluster_exists() check in stop-test-cluster.py. Anything unusual (help,
# an unknown argument, or an invalid id) is left to the Python script.
cluster_id="${CLUSTER_ID:-tools-test}"
dsc=false
defer=false
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
    case "${args[i]}" in
    -h | --help) defer=true ;;
    --dsc) dsc=true ;;
    --id=*) cluster_id="${args[i]#--id=}" ;;
    --id)
        cluster_id="${args[i + 1]:-}"
        i=$((i + 1))
        ;;
    *) defer=true ;;
    esac
done
if [ "$dsc" = true ]; then
    cluster_id=tools-dsc
fi
if [ "$defer" = false ] && [[ $cluster_id =~ ^[A-Za-z0-9_-]+$ ]]; then
    cluster_dir="$(cd "${SCRIPT_DIR:?}/.." && pwd)/test-cluster/$cluster_id"
    if [ ! -f "$cluster_dir/connection-string" ] && [ ! -d "$cluster_dir/logs" ]; then
        echo "no $cluster_dir to stop" >&2
        exit 0
    fi
fi

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

exec python3 "${SCRIPT_DIR:?}/stop-test-cluster.py" "$@"
