#!/bin/bash

# Starts a MongoDB cluster with mongodb-runner and prints the connection string to export as
# TOOLS_TESTING_MONGOD.
#
# The runner allocates free ports, so there is no fixed localhost:33333 any more. Export the URI the
# script prints, then run the integration tests.
#
# Usage:
#   ./scripts/start-test-cluster.sh [options] [-- <extra mongod args>]
#
# An option with a value can be written as either "--opt value" or "--opt=value".
#
# Options:
#   --topology=standalone|replset|sharded   Cluster topology (default standalone)
#   --version=<ver>                         Server version for mongodb-runner to download (default 8.0)
#   --bin-dir=<dir>                         Use existing mongod/mongos binaries instead of downloading
#   --id=<id>                               Cluster id, also used by stop (default tools-test)
#   --shards=<n>                            Number of shards for the sharded topology (default 1)
#   --tls                                   Require TLS using the certificates under common/db/testdata
#   --create-user                           Create an admin user for auth runs (needs AUTH_USERNAME and
#                                           AUTH_PASSWORD)
#   --evg-expansion-var=<name>              Write an Evergreen expansions file setting name to the URI
#
# Every option can also be set through the matching environment variable (TOPOLOGY, VERSION, BIN_DIR,
# CLUSTER_ID, SHARDS, USE_TLS, CREATE_USER, AUTH_USERNAME, AUTH_PASSWORD, EVG_EXPANSION_VAR,
# ADDITIONAL_ARGS). CI uses the environment form so that the auth password is not passed on the
# command line.
#
# Stop the cluster with scripts/stop-test-cluster.sh.

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
SHARDS="${SHARDS:-1}"
USE_TLS="${USE_TLS:-false}"
CREATE_USER="${CREATE_USER:-false}"
AUTH_USERNAME="${AUTH_USERNAME:-}"
AUTH_PASSWORD="${AUTH_PASSWORD:-}"
EVG_EXPANSION_VAR="${EVG_EXPANSION_VAR:-}"
ADDITIONAL_ARGS="${ADDITIONAL_ARGS:-}"
EXTRA_MONGOD_ARGS=()

usage() {
    sed -n '3,30p' "$0" | sed 's/^# \{0,1\}//'
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

# Reuse the binaries that scripts/download-mongod-and-shell.sh drops in ./bin when they are there so
# that unreleased Server builds work without a second download path.
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

# internalClientOptions and users are not CLI flags, so they have to go through --config. We pass the
# repo's own test certificates so that the runner's URI carries the same TLS options the tests build
# from TOOLS_TESTING_SSL; tlsAddClientKey=false stops the runner from replacing them with a generated
# cert. The server certificate is valid for 127.0.0.1, so we do not need tlsAllowInvalidCertificates;
# adding it would put an unsupported parameter in the URI, which the tools log as a warning.
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

# The runner writes diagnostics to stderr and only the connection string to stdout, so piping stdout
# through tee keeps the diagnostics visible while leaving the URI parseable.
OUTPUT="$("${RUNNER[@]}" "${RUNNER_ARGS[@]}" | tee /dev/stderr)"

# The grep can legitimately fail if the cluster came up but the URI is missing, so the `if !` is
# what keeps errexit from swallowing the diagnostic below.
if ! CONNECTION_STRING="$(echo "${OUTPUT:?}" | grep -oE 'mongodb://[^ ]+' | tail -1)"; then
    echo "the cluster started but printed no mongodb:// connection string; check ${LOG_DIR}" >&2
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
