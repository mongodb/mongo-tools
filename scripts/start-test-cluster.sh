#!/bin/bash

# Starts a MongoDB cluster with mongodb-runner for local development and prints the connection
# string that you should export as `TOOLS_TESTING_MONGOD`.
#
# The runner picks free ports randomly.
#
# Usage:
#   ./scripts/start-test-cluster.sh [options] [-- <extra mongod args>]
#
# An option with a value can be written as either "--opt value" or "--opt=value".
#
# Options:
#   --topology standalone|replset|sharded   Cluster topology (default standalone)
#   --version <ver>                         Server version for mongodb-runner to download (default 8.0)
#   --bin-dir <dir>                         Use existing mongod/mongos binaries instead of downloading
#   --id <id>                               Cluster id, also used by stop (default tools-test)
#   --shards <n>                            Number of shards for the sharded topology (default 2)
#   --tls                                   Require TLS using the certificates under common/db/testdata
#
# Stop the cluster with scripts/stop-test-cluster.sh.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"

TOPOLOGY=standalone
VERSION=8.0
BIN_DIR=
CLUSTER_ID=tools-test
SHARDS=2
USE_TLS=false
EXTRA_MONGOD_ARGS=()

usage() {
    sed -n '3,22p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
    # Accept both "--opt value" and "--opt=value". Bash's built-in getopts only handles short
    # options, and GNU getopt (which does handle long options) is not available on macOS, so rewrite
    # the separated form to the --opt=value form the parser below expects.
    case "$1" in
    --topology | --version | --bin-dir | --id | --shards)
        if [ $# -lt 2 ]; then
            echo "option $1 requires a value" >&2
            exit 1
        fi
        set -- "$1=$2" "${@:3}"
        ;;
    esac

    case "$1" in
    --topology=*) TOPOLOGY="${1#*=}" ;;
    --version=*) VERSION="${1#*=}" ;;
    --bin-dir=*) BIN_DIR="${1#*=}" ;;
    --id=*) CLUSTER_ID="${1#*=}" ;;
    --shards=*) SHARDS="${1#*=}" ;;
    --tls) USE_TLS=true ;;
    --)
        shift
        EXTRA_MONGOD_ARGS=("$@")
        break
        ;;
    --help | -h)
        usage
        exit 0
        ;;
    *)
        echo "unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
done

case "$TOPOLOGY" in
standalone | replset | sharded) ;;
*)
    echo "invalid topology '$TOPOLOGY'; expected standalone, replset, or sharded" >&2
    exit 1
    ;;
esac

REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"

# We use the binaries in `./bin` if they already exist. This lets us test arbitrary Server binaries,
# not just the ones downloaded by mongodb-downloader.
EXTENSION=
if [ -f "${REPO_ROOT:?}/bin/mongod.exe" ]; then
    EXTENSION=.exe
fi
if [ -z "$BIN_DIR" ] && [ -f "${REPO_ROOT:?}/bin/mongod${EXTENSION:-}" ]; then
    BIN_DIR="${REPO_ROOT:?}/bin"
fi

RUNNER=(mise exec node npm:@mongodb-js/mongodb-runner -- mongodb-runner)

CLUSTER_DIR="${REPO_ROOT:?}/test-cluster/${CLUSTER_ID:?}"
LOG_DIR="${CLUSTER_DIR:?}/logs"
mkdir -p "${LOG_DIR:?}"

RUNNER_ARGS=(
    start
    "--topology=${TOPOLOGY:?}"
    "--id=${CLUSTER_ID:?}"
    "--logDir=${LOG_DIR:?}"
)

if [ -n "$BIN_DIR" ]; then
    RUNNER_ARGS+=("--binDir=${BIN_DIR}")
else
    RUNNER_ARGS+=("--version=${VERSION:?}")
fi

if [ "$TOPOLOGY" = "sharded" ]; then
    RUNNER_ARGS+=("--shards=${SHARDS:?}")
fi

MONGOD_ARGS=("${EXTRA_MONGOD_ARGS[@]}")
if [ "$USE_TLS" = "true" ]; then
    MONGOD_ARGS+=(
        --tlsMode requireTLS
        --tlsCAFile "${REPO_ROOT:?}/common/db/testdata/ca-ia.pem"
        --tlsCertificateKeyFile "${REPO_ROOT:?}/common/db/testdata/test-server.pem"
    )
fi

if [ "${#MONGOD_ARGS[@]}" -gt 0 ]; then
    RUNNER_ARGS+=(--)
    RUNNER_ARGS+=("${MONGOD_ARGS[@]}")
fi

# Temporary workaround for mongodb-runner hanging against MongoDB 4.2, which logs in the legacy text
# format; see scripts/mongodb-runner-legacy-log-shim.js. The patch is a no-op for 4.4+. Remove this
# and the shim once the upstream fix lands.
SHIM="${REPO_ROOT:?}/scripts/mongodb-runner-legacy-log-shim.js"
if command -v cygpath >/dev/null 2>&1; then
    SHIM="$(cygpath -w "$SHIM")"
fi
export NODE_OPTIONS="--require=${SHIM}${NODE_OPTIONS:+ $NODE_OPTIONS}"

# The runner writes its diagnostics to stderr (which the task log already captures) and only the
# connection string to stdout, so we just capture stdout. We don't pipe through `tee /dev/stderr`:
# that path doesn't exist under Cygwin, so tee dies and the command substitution hangs.
OUTPUT="$("${RUNNER[@]}" "${RUNNER_ARGS[@]}")"

# The grep can legitimately fail if the cluster came up but the URI is missing, so the `if !` is
# what keeps errexit from swallowing the diagnostic below.
if ! CONNECTION_STRING="$(echo "${OUTPUT:?}" | grep -oE 'mongodb://[^ ]+' | tail -1)"; then
    echo "the cluster started but printed no mongodb:// connection string; check ${LOG_DIR}" >&2
    exit 1
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
