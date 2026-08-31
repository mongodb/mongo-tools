#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck disable=SC1091
source "$SCRIPT_DIR/functions.sh"

# Evergreen's ${workdir} expansion on Windows sometimes lacks a drive letter (e.g. "\data\mci\9f92"),
# and Go's os/exec refuses to run an executable found via a PATH entry it can't prove is absolute.
# cygpath -am always produces an absolute path with a drive letter.
case "$(uname -s)" in
CYGWIN* | MINGW* | MSYS*)
    EVG_WORKDIR=$(cygpath -am "${EVG_WORKDIR:?}")
    ;;
esac

export PATH="${EVG_WORKDIR:?}/.local/bin:$PATH"

export MISE_DATA_DIR="${EVG_WORKDIR:?}/.local/share/mise"

# cache.save dereferences symlinks (see the comment in common.yml for why we can't turn that off),
# so the bin/<tool> -> ../lib/node_modules/<pkg>/... links that npm creates come back as plain
# copies sitting in bin/. Node then resolves a tool's relative requires against bin/ instead of the
# package directory, and every npm-backed tool dies with MODULE_NOT_FOUND. Recreating the links
# makes the cache-hit tree behave like a fresh install.
recreate_npm_bin_symlinks() {
    local mise_data_dir
    mise_data_dir="${EVG_WORKDIR:?}/.local/share/mise"

    for install_dir in "${mise_data_dir}/installs"/npm-*/*/; do
        local lib_modules="${install_dir}lib/node_modules"
        [ -d "${lib_modules}" ] || continue
        for pkg_dir in "${lib_modules}"/*/; do
            [ -f "${pkg_dir}package.json" ] || continue
            local pkg_name
            pkg_name=$(basename "${pkg_dir}")
            python3 "$SCRIPT_DIR/recreate-npm-bin-symlinks.py" "${install_dir%/}" "${pkg_name}" "${pkg_dir}package.json"
        done
    done
}

recreate_npm_bin_symlinks

# Cache hit: .local/bin and .local/share/mise were already restored from S3, so every tool is
# present and there's nothing to install.
if [ "${MISE_ALL_TOOLS_CACHE_HIT:-}" = "true" ]; then
    exit 0
fi

# We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install
