#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

WORKDIR="$1"

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

MISE_BIN_DIR="${workdir:?}/.local/bin"
export PATH="$MISE_BIN_DIR:$PATH"

WANT_MISE_VERSION=$(cat "$REPO_ROOT/scripts/mise-version.txt")

# Install mise if it is not present or is the wrong version.
INSTALL_MISE="yes"
set +o errexit
MISE_PATH=$(which mise 2>/dev/null)
set -o errexit

if [ -n "$MISE_PATH" ]; then
    MISE_VERSION_OUTPUT=$(mise --version)
    if [[ "$MISE_VERSION_OUTPUT" == *"$WANT_MISE_VERSION"* ]]; then
        echo "mise $WANT_MISE_VERSION is already installed, skipping installation"
        INSTALL_MISE=""
    else
        echo "Found mise $MISE_VERSION_OUTPUT but want $WANT_MISE_VERSION, reinstalling"
    fi
else
    echo "mise not found, installing"
fi

if [ -n "$INSTALL_MISE" ]; then
    MISE_INSTALL_HELP=0 \
        MISE_INSTALL_PATH="$MISE_BIN_DIR/mise" \
        MISE_INSTALL_MUSL=1 \
        "$REPO_ROOT/etc/mise.run.sh"
fi

mise settings experimental=true

export MISE_DATA_DIR="${WORKDIR:?}/.local/share/mise"

set +e
mise where go
STATUS=$?
set -e

# If the status is not 0, that means the version of Go defined in the `mise.toml` file is not
# installed.
if [ $STATUS -ne 0 ]; then
    mise install go
fi

mise exec go -- go version
mise exec go -- go env

mise exec node -- npm install
