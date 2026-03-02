#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

MISE_BIN_DIR="${workdir:?}/.local/bin"
export PATH="$MISE_BIN_DIR:$PATH"

WANT_MISE_VERSION=$(cat "$REPO_ROOT/scripts/mise-version.txt")

# Install mise if it is not present or is the wrong version.
INSTALL_MISE="yes"
set +e
MISE_PATH=$(which mise 2>/dev/null)
set -e

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

mise install

mise exec node -- npm install
