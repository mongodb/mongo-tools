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
