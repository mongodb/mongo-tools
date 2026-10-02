#!/bin/bash

#----------------------------------------------------------------------
# This script exposes functionality that’s useful across various
# shell scripts.
#
# Feel free to augment it as appropriate.
#----------------------------------------------------------------------

# A simple shell function to retry a given command until it succeeds.
#
# This is adapted from:
# https://stackoverflow.com/questions/7449772/how-to-retry-a-command-in-bash
#
# It waits a bit between tries; if enough attempts fail, then it increases
# the delay between attempts. If enough attempts fail thereafter, then the
# function fails.
retry() {

    # Initial delay between attempts:
    retry_delay=5

    # Number of failures before we back off:
    failures_before_backoff=${RETRY_FAILURES_BEFORE_BACKOFF:-10}

    # Delay between attempts after backoff:
    backoff_retry_delay=30

    # Number of failures before we fail:
    failures_before_hard_fail=${RETRY_FAILURES_BEFORE_HARD_FAIL:-20}

    failures=0

    until "$@"; do
        failures=$((failures + 1))

        if [[ $failures -eq $failures_before_backoff ]]; then
            retry_delay=$backoff_retry_delay
            echo "Attempt interval increased to $retry_delay seconds."
        elif [[ $failures -eq $failures_before_hard_fail ]]; then
            echo "Too many failures; we’ll try one last time …"
            break
        fi

        echo Sleeping $retry_delay seconds before retrying …
        sleep $retry_delay
    done

    if [[ $failures -eq $failures_before_hard_fail ]]; then
        "$@"
    fi
}

# cache.save dereferences symlinks (see the comment in common.yml for why we can't turn that off), so
# the bin/<tool> -> ../lib/node_modules/<pkg>/... links that npm creates come back as plain copies
# sitting in bin/. Node then resolves a tool's relative requires against bin/ instead of the package
# directory, and every npm-backed tool dies with MODULE_NOT_FOUND. Recreating the links makes a
# cache-hit tree behave like a fresh install.
recreate_npm_bin_symlinks() {
    local script_dir mise_data_dir
    script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
    mise_data_dir="${EVG_WORKDIR:?}/.local/share/mise"

    for install_dir in "${mise_data_dir}/installs"/npm-*/*/; do
        local lib_modules="${install_dir}lib/node_modules"
        [ -d "${lib_modules}" ] || continue
        # A plain glob over node_modules/* misses scoped packages, which live one level deeper at
        # node_modules/@scope/pkg, so find the package.json files instead.
        while IFS= read -r pkg_json; do
            local pkg_name
            pkg_name="${pkg_json#"${lib_modules}"/}"
            pkg_name="${pkg_name%/package.json}"
            python3 "$script_dir/recreate-npm-bin-symlinks.py" "${install_dir%/}" "${pkg_name}" "${pkg_json}"
        done < <(find "${lib_modules}" -mindepth 2 -maxdepth 3 -name package.json -type f)
    done
}
