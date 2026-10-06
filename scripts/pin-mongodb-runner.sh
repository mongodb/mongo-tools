#!/bin/bash

# Pins the mongodb driver bundled with mongodb-runner to 7.5.0. The runner declares `mongodb:
# ^7.5.0`, but a fresh install resolves to a newer 7.x that dropped MongoDB 4.2 support (its Node
# driver requires wire version 9 / MongoDB 4.4). 7.5.0 is the last 7.x that supports 4.2, which our
# 4.2 test tasks need. This runs as the mise postinstall hook (see mise.toml) and is also called by
# scripts/install-mongodb-runner.sh. Remove it once mongodb-runner supports 4.2 itself.

set -o errexit
set -o pipefail

# A missing install is not an error: the hook runs on every `mise install`, including ones that
# don't install the runner.
INSTALL_DIR="$(mise where npm:@mongodb-js/mongodb-runner 2>/dev/null || true)"

if [ -z "$INSTALL_DIR" ] || [ ! -d "$INSTALL_DIR" ]; then
    exit 0
fi

# The npm backend has used both a flat layout (node_modules/...) and an older lib/node_modules one.
RUNNER_PKG=
for candidate in \
    "${INSTALL_DIR:?}/node_modules/@mongodb-js/mongodb-runner" \
    "${INSTALL_DIR:?}/lib/node_modules/@mongodb-js/mongodb-runner"; do
    if [ -d "$candidate" ]; then
        RUNNER_PKG="$candidate"
        break
    fi
done

if [ -z "$RUNNER_PKG" ]; then
    exit 0
fi

if grep -q '"version": "7.5.0"' "$RUNNER_PKG/node_modules/mongodb/package.json" 2>/dev/null; then
    exit 0
fi

# Install just mongodb and its dependencies in a scratch directory, then drop them into the runner's
# node_modules. Installing in the runner's own directory would reinstall all of the runner's
# dependencies too, which we don't want.
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

(
    cd "$tmp_dir"
    mise exec node -- npm install --no-save --no-package-lock --no-audit --no-fund --ignore-scripts mongodb@7.5.0
)

mkdir -p "$RUNNER_PKG/node_modules"
# Not `cp -a`: under Cygwin the permission-preservation flags are unsupported and cp exits non-zero,
# which would fail the install.
cp -R "$tmp_dir"/node_modules/. "$RUNNER_PKG/node_modules/"
