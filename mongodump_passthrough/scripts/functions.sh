#!/bin/bash
# this file is copied from mongosync/etc/functions.sh

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

print_resmoke_version_for_source_server_version() {
    source_server_version=$1

    case "$source_server_version" in
    v80 | 80 | 8.0)
        echo 8.0
        ;;
    v70 | 70 | 7.0)
        echo 7.0
        ;;
    v60 | 60 | 6.0 | v50 | 50 | 5.0 | v44 | 44 | 4.4)
        echo 6.0
        ;;
    *)
        echo "Unknown source server version: [$source_server_version]" >&2
        exit 1
        ;;
    esac
}

export_server_poetry_requirements() {
    python="$1"

    # As of 2025-07, this ends up exporting a list of requirements with two versions each of
    # cryptography and pygithub, then pip complains it can't install both. So we need to filter out
    # one of the two. And to filter them out we need the export to not include hashes. That's
    # because when it includes hashes, the output from `poetry export` for each package spans
    # multiple lines, which makes it much harder to use grep.
    "$python" -m poetry export --with aws,core,evergreen,testing --without-hashes | grep -v -F 'cryptography==2.3' | grep -v -F 'pygithub==1.58.0'
}

install_server_pip_modules() {
    python="$1"
    if [ -z "$python" ]; then
        echo "Give the Python executable path."
        exit 1
    fi

    "$python" -m pip --disable-pip-version-check install "pip==21.0.1" "wheel==0.37.0" || exit 1

    # Resmoke v8 uses Poetry; prior versions used a dev-requirements.txt file.
    if [ -e poetry.lock ]; then
        echo "Poetry file detected. Installing Poetry ..."
        "$python" -m pip --disable-pip-version-check install poetry poetry-plugin-export

        echo "Generating requirements file ..."
        requirements_blob=$(export_server_poetry_requirements python)
    else
        echo "Reading Python requirements file from server repo ..."

        # The dev-requirements are necessary to include mongo-tooling-metrics.
        requirements_blob=$(cat etc/pip/dev-requirements.txt)
    fi

    echo "Installing Python modules ..."
    if ! echo "$requirements_blob" | "$python" -m pip --disable-pip-version-check install --requirement /dev/stdin --log install.log; then
        echo "Pip install error"
        cat install.log
        exit 1
    fi
}
