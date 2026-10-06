#!/bin/bash

# Starts a MongoDB cluster with mongodb-runner for local development and prints the connection
# string that you should export as `TOOLS_TESTING_MONGOD`. Stop it with scripts/stop-test-cluster.sh.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT="$(cd "${SCRIPT_DIR:?}/.." && pwd)"

# In Evergreen, mise itself is under ${workdir}, which is what ci-env.sh puts on PATH.
if [ -n "${EVG_WORKDIR:-}" ]; then
    # shellcheck source=scripts/ci-env.sh
    source "${SCRIPT_DIR:?}/ci-env.sh"
fi

TOPOLOGY="${TOPOLOGY:-standalone}"
VERSION="${VERSION:-8.0}"
BIN_DIR="${BIN_DIR:-}"
CLUSTER_ID="${CLUSTER_ID:-tools-test}"
SHARDS="${SHARDS:-2}"
USE_TLS="${USE_TLS:-false}"
CREATE_USER="${CREATE_USER:-false}"
AUTH_USERNAME="${AUTH_USERNAME:-}"
AUTH_PASSWORD="${AUTH_PASSWORD:-}"
EVG_EXPANSION_VAR="${EVG_EXPANSION_VAR:-}"
ADDITIONAL_ARGS="${ADDITIONAL_ARGS:-}"
EXTRA_MONGOD_ARGS=()

usage() {
    cat <<'EOF'
Starts a MongoDB cluster with mongodb-runner and prints the connection string that you should export
as `TOOLS_TESTING_MONGOD`.

The runner picks free ports randomly.

Usage:
  ./scripts/start-test-cluster.sh [options] [-- <extra mongod args>]

An option with a value can be written as either "--opt value" or "--opt=value".

Options:
  --topology standalone|replset|sharded   Cluster topology (default standalone)
  --version <ver>                         Server version for mongodb-runner to download (default 8.0)
  --bin-dir <dir>                         Use existing mongod/mongos binaries instead of downloading
  --id <id>                               Cluster id, also used by stop (default tools-test)
  --shards <n>                            Number of shards for the sharded topology (default 2)
  --tls                                   Require TLS using the certificates under common/db/testdata
  --create-user                           Create an admin user for auth runs (needs AUTH_USERNAME and
                                          AUTH_PASSWORD)
  --evg-expansion-var=<name>              Write an Evergreen expansions file setting name to the URI

Every option can also be set through the matching environment variable (TOPOLOGY, VERSION, BIN_DIR,
CLUSTER_ID, SHARDS, USE_TLS, CREATE_USER, AUTH_USERNAME, AUTH_PASSWORD, EVG_EXPANSION_VAR,
ADDITIONAL_ARGS). CI uses the environment form so that the auth password is not passed on the
command line.

Stop the cluster with scripts/stop-test-cluster.sh.
EOF
}

while [ $# -gt 0 ]; do
    # Accept both "--opt value" and "--opt=value". Bash's built-in getopts only handles short
    # options, and GNU getopt (which does handle long options) is not available on macOS, so rewrite
    # the separated form to the --opt=value form the parser below expects.
    case "$1" in
    --topology | --version | --bin-dir | --id | --shards | --evg-expansion-var)
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
    --create-user) CREATE_USER=true ;;
    --evg-expansion-var=*) EVG_EXPANSION_VAR="${1#*=}" ;;
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

# mongodb-runner rejects ids outside [A-Za-z0-9_-], but we build filesystem paths from the id before
# it gets that far, so validate it up front.
case "$CLUSTER_ID" in
*[!A-Za-z0-9_-]* | "")
    echo "invalid cluster id '$CLUSTER_ID'; use only letters, digits, underscores, and hyphens" >&2
    exit 1
    ;;
esac

if [ "$CREATE_USER" = "true" ] && { [ -z "$AUTH_USERNAME" ] || [ -z "$AUTH_PASSWORD" ]; }; then
    echo "CREATE_USER=true requires both AUTH_USERNAME and AUTH_PASSWORD" >&2
    exit 1
fi

# mongodb-runner and mongod are native binaries, so under Cygwin they cannot open the POSIX paths
# we use everywhere else. Translate just the arguments they see; bash keeps using the POSIX paths.
native_path() {
    if command -v cygpath >/dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s' "$1"
    fi
}

CA_FILE="${REPO_ROOT:?}/common/db/testdata/ca-ia.pem"
CERT_FILE="${REPO_ROOT:?}/common/db/testdata/test-server.pem"
CA_FILE_NATIVE="$(native_path "${CA_FILE:?}")"
CERT_FILE_NATIVE="$(native_path "${CERT_FILE:?}")"

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
    "--logDir=$(native_path "${LOG_DIR:?}")"
)

if [ -n "$BIN_DIR" ]; then
    RUNNER_ARGS+=("--binDir=$(native_path "${BIN_DIR:?}")")
else
    RUNNER_ARGS+=("--version=${VERSION:?}")
fi

if [ "$TOPOLOGY" = "sharded" ]; then
    RUNNER_ARGS+=("--shards=${SHARDS:?}")
fi

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

# `internalClientOptions` and users are not CLI flags, so they have to go through `--config`. We
# pass the repo's own test certificates so that the runner's URI has the same TLS options the tests
# build from `TOOLS_TESTING_SSL`. Setting `tlsAddClientKey=false` stops the runner from replacing
# them with a generated cert. The server certificate is valid for 127.0.0.1, so we do not need
# tlsAllowInvalidCertificates; adding it would put an unsupported parameter in the URI, which the
# tools log as a warning.
CONFIG_ENTRIES=()
if [ "$USE_TLS" = "true" ]; then
    CONFIG_ENTRIES+=('"tlsAddClientKey": false')
    CONFIG_ENTRIES+=(
        "\"internalClientOptions\": {\"tls\": true, \"tlsCAFile\": \"$(json_escape "${CA_FILE_NATIVE:?}")\", \"tlsCertificateKeyFile\": \"$(json_escape "${CERT_FILE_NATIVE:?}")\"}"
    )
fi
if [ "$CREATE_USER" = "true" ]; then
    CONFIG_ENTRIES+=(
        "\"users\": [{\"username\": \"$(json_escape "${AUTH_USERNAME:?}")\", \"password\": \"$(json_escape "${AUTH_PASSWORD:?}")\", \"roles\": [{\"role\": \"__system\", \"db\": \"admin\"}]}]"
    )
fi

if [ "${#CONFIG_ENTRIES[@]}" -gt 0 ]; then
    CONFIG_FILE="${CLUSTER_DIR:?}/runner-config.json"
    CONFIG_JOINED=
    for entry in "${CONFIG_ENTRIES[@]}"; do
        if [ -n "$CONFIG_JOINED" ]; then
            CONFIG_JOINED+=","$'\n'
        fi
        CONFIG_JOINED+="$entry"
    done
    printf '{\n%s\n}\n' "$CONFIG_JOINED" >"${CONFIG_FILE:?}"
    RUNNER_ARGS+=(--config "$(native_path "${CONFIG_FILE:?}")")
fi

MONGOD_ARGS=("${EXTRA_MONGOD_ARGS[@]}")
if [ -n "$ADDITIONAL_ARGS" ]; then
    # shellcheck disable=SC2206 # ADDITIONAL_ARGS is intentionally word-split
    MONGOD_ARGS+=($ADDITIONAL_ARGS)
fi
if [ "$USE_TLS" = "true" ]; then
    MONGOD_ARGS+=(
        --tlsMode requireTLS
        --tlsCAFile "${CA_FILE_NATIVE:?}"
        --tlsCertificateKeyFile "${CERT_FILE_NATIVE:?}"
    )
fi
if [ "$CREATE_USER" = "true" ]; then
    MONGOD_ARGS+=(--auth)
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

# The runner prints the connection string once the cluster is up, but on Windows it does not exit:
# the detached mongod keeps its Node event loop alive. Capturing stdout with `$(...)` would then wait
# forever for the pipe to close (and `tee /dev/stderr` is worse, as that path doesn't exist under
# Cygwin). So run the runner in the background with its output in a file and poll for the URI.
RUNNER_OUTPUT="${CLUSTER_DIR:?}/runner-start.log"
: >"${RUNNER_OUTPUT:?}"
"${RUNNER[@]}" "${RUNNER_ARGS[@]}" </dev/null >>"${RUNNER_OUTPUT:?}" 2>&1 &
RUNNER_PID=$!

# The runner writes its state file before printing the URI, so once we see the URI the cluster is
# registered and it's safe to stop waiting for the process.
CONNECTION_STRING=
for ((attempt = 0; attempt < 900; attempt++)); do
    CONNECTION_STRING="$(grep -oE 'mongodb://[^[:space:]]+' "${RUNNER_OUTPUT:?}" 2>/dev/null | tail -1 || true)"
    if [ -n "$CONNECTION_STRING" ]; then
        break
    fi
    if ! kill -0 "$RUNNER_PID" 2>/dev/null; then
        break
    fi
    sleep 1
done

# Best-effort. The runner never stops the cluster on a signal, so this can't take the cluster down,
# and we don't `wait` because on Windows the kill may not land.
kill "$RUNNER_PID" 2>/dev/null || true

if [ -z "$CONNECTION_STRING" ]; then
    echo "the cluster did not start; check ${RUNNER_OUTPUT} and ${LOG_DIR}" >&2
    tail -n 50 "${RUNNER_OUTPUT:?}" >&2 || true
    exit 1
fi

# The tools fall back to the operational database as the auth source when the URI does not name one,
# which fails authentication for a user created in admin, so make the source explicit.
if [ "$CREATE_USER" = "true" ]; then
    case "$CONNECTION_STRING" in
    *authSource=*) ;;
    *\?*) CONNECTION_STRING="${CONNECTION_STRING}&authSource=admin" ;;
    *) CONNECTION_STRING="${CONNECTION_STRING}?authSource=admin" ;;
    esac
fi

echo "${CONNECTION_STRING:?}" >"${CLUSTER_DIR:?}/connection-string"

# The CI "start test cluster" function runs expansions.update against this file so later commands see
# TOOLS_TESTING_MONGOD.
if [ -n "$EVG_EXPANSION_VAR" ]; then
    echo "${EVG_EXPANSION_VAR}: '${CONNECTION_STRING:?}'" >"${CLUSTER_DIR:?}/expansions.yml"
fi

echo
echo "export TOOLS_TESTING_MONGOD='${CONNECTION_STRING:?}'"
