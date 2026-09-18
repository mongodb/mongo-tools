#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"
# shellcheck source=scripts/functions.sh
. "$SCRIPT_DIR/functions.sh"

: "${MONGO_VERSION:?}"

REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# mongodb-downloader lives in a private repo, so it is in mise.dsc.toml rather than mise.toml and
# needs MISE_ENV=dsc to be visible to mise and a token to read that repo. We only retry twice because
# the likely failure is a token that cannot read the repo, which is not transient.
export MISE_ENV=dsc
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install github:10gen/mongodb-downloader

# The config labels versions by major.minor; MONGO_VERSION can be a full version or a release
# candidate, and "latest" is a label of its own.
case "$MONGO_VERSION" in
latest) DOWNLOADER_LABEL="latest" ;;
*) DOWNLOADER_LABEL="$(echo "$MONGO_VERSION" | cut -d. -f1,2)" ;;
esac

# `download --to` replaces its target directory wholesale, and ./bin also holds the built tools
# binaries, so we extract into a scratch directory and merge the Server binaries into ./bin.
SERVER_DIR="${REPO_ROOT:?}/.server-download"
CONFIG_PATH="${REPO_ROOT:?}/etc/mongodb-downloader-config.yaml"
OUTPUT_DIR="${REPO_ROOT:?}/etc"
TO_ARG="$SERVER_DIR"

# mongodb-downloader is a native Windows binary, so under Cygwin it cannot open the POSIX paths we
# use everywhere else. Translate just the arguments it sees; the mv below still uses the POSIX path.
if command -v cygpath >/dev/null 2>&1; then
    CONFIG_PATH="$(cygpath -w "$CONFIG_PATH")"
    OUTPUT_DIR="$(cygpath -w "$OUTPUT_DIR")"
    TO_ARG="$(cygpath -w "$TO_ARG")"
fi

rm -rf "$SERVER_DIR"
mise exec github:10gen/mongodb-downloader -- mongodb-downloader download \
    --config "$CONFIG_PATH" \
    --output-dir "$OUTPUT_DIR" \
    --server-version "$DOWNLOADER_LABEL" \
    --to "$TO_ARG"

mkdir -p "${REPO_ROOT:?}/bin"
mv "$SERVER_DIR"/bin/* "${REPO_ROOT:?}/bin/"
rm -rf "$SERVER_DIR"
chmod +x "${REPO_ROOT:?}"/bin/*

# The jstestshell is not something mongodb-downloader handles; the legacy JS tests still need it.
$GO_EXEC_PREFIX go run release/release.go download-shell
