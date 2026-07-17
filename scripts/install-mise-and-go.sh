#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
# shellcheck disable=SC1091
source "$SCRIPT_DIR/functions.sh"

# mise's own install script (etc/mise.run.sh) only supports macOS and Linux. On Windows, Evergreen
# runs `shell.exec` under Cygwin, so we download mise's Windows release zip directly instead. We use
# PowerShell's Expand-Archive to unpack it rather than `unzip`, since Cygwin doesn't reliably have
# `unzip` installed.
install_mise_windows() {
    local zip_name="mise-v${WANT_MISE_VERSION}-windows-x64.zip"
    local url="https://github.com/jdx/mise/releases/download/v${WANT_MISE_VERSION}/${zip_name}"
    local tmp_zip tmp_extract
    tmp_zip=$(mktemp --suffix=.zip)
    tmp_extract=$(mktemp -d)

    retry curl --fail --location --silent --show-error --output "$tmp_zip" "$url"

    powershell.exe -NoProfile -Command \
        "Expand-Archive -Path '$(cygpath -w "$tmp_zip")' -DestinationPath '$(cygpath -w "$tmp_extract")' -Force"

    mkdir -p "$MISE_BIN_DIR"
    cp "$tmp_extract/mise/bin/mise.exe" "$MISE_BIN_DIR/mise.exe"
    cp "$tmp_extract/mise/bin/mise-shim.exe" "$MISE_BIN_DIR/mise-shim.exe"
}

# Cache hit: mise + Go were already restored from S3, nothing to do.
if [ "${MISE_AND_GO_CACHE_HIT:-}" = "true" ]; then
    exit 0
fi

# Evergreen's ${workdir} expansion on Windows sometimes lacks a drive letter
# (e.g. "\data\mci\9f92"), and Go's os/exec refuses to run an executable found via a PATH entry it
# can't prove is absolute. cygpath -am always produces an absolute path with a drive letter. We
# also set GOCACHE here since `go env`/`go version`'s default lookup (via %LocalAppData%) doesn't
# reliably resolve to an absolute path under Cygwin either.
case "$(uname -s)" in
CYGWIN* | MINGW* | MSYS*)
    EVG_WORKDIR=$(cygpath -am "${EVG_WORKDIR:?}")
    export GOCACHE="C:/windows/temp"
    ;;
esac

MISE_BIN_DIR="${EVG_WORKDIR:?}/.local/bin"
export PATH="$MISE_BIN_DIR:$PATH"

WANT_MISE_VERSION=$(cat "$REPO_ROOT/scripts/mise-version.txt")

# Install mise if it is not present or is the wrong version.
INSTALL_MISE="yes"
set +o errexit
MISE_PATH=$(which mise 2>/dev/null)
set -o errexit

if [ -n "$MISE_PATH" ]; then
    MISE_VERSION_OUTPUT=$(mise --version)
    if [[ $MISE_VERSION_OUTPUT == *"$WANT_MISE_VERSION"* ]]; then
        echo "mise $WANT_MISE_VERSION is already installed, skipping installation"
        INSTALL_MISE=""
    else
        echo "Found mise $MISE_VERSION_OUTPUT but want $WANT_MISE_VERSION, reinstalling"
    fi
else
    echo "mise not found, installing"
fi

if [ -n "$INSTALL_MISE" ]; then
    case "$(uname -s)" in
    CYGWIN* | MINGW* | MSYS*)
        install_mise_windows
        ;;
    *)
        # We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
        RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
            MISE_INSTALL_HELP=0 \
            MISE_INSTALL_PATH="$MISE_BIN_DIR/mise" \
            MISE_INSTALL_MUSL=1 \
            retry "$REPO_ROOT/etc/mise.run.sh"
        ;;
    esac
fi

export MISE_DATA_DIR="${EVG_WORKDIR:?}/.local/share/mise"

# We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install go

mise exec go -- go version
mise exec go -- go env
