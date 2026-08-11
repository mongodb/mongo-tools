#!/bin/bash

# Tears down a DSC cluster started by scripts/start-dsc-cluster.sh, including the docker compose
# project providing the SLS storage backend. See TOOLS-4100.

set -o errexit
set -o nounset
set -o pipefail

: "${DEVTOOLS_SHARED:?path to a checkout of devtools-shared PR 822 (DSC support)}"

CLUSTER_ID="tools-dsc"
# Derived the same way start-dsc-cluster.sh derives it, so this works from any working directory.
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

node "${DEVTOOLS_SHARED:?}/packages/mongodb-runner/bin/runner.js" stop --id="${CLUSTER_ID:?}"
rm -f "${REPO_ROOT:?}/dsc-cluster/connection-string"
