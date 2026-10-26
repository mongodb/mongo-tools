#!/bin/bash

# Starts a MongoDB cluster with mongodb-runner for local development and prints the connection
# string that you should export as `TOOLS_TESTING_MONGOD`. Stop it with scripts/stop-test-cluster.sh.
#
# The logic lives in start-test-cluster.py. This wrapper keeps Evergreen's `binary: bash` invocation
# (Windows cannot execute a shebang script as a native binary) and inherits the environment ci-env.sh
# sets up, notably mise on PATH.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

exec python3 "${SCRIPT_DIR:?}/start-test-cluster.py" "$@"
