#!/bin/bash

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=evergreen/scripts/resmoke_venv_activate.sh
. "$SCRIPT_DIR/resmoke_venv_activate.sh"

set -o errexit
set -o verbose

resmoke_venv_activate
# shellcheck disable=SC2154
cd "$resmoke_dir" || exit 1

# Retry this up to 3 times because there can occasionally
# be dependency issues.
set +e
for i in {1..3}; do
    # This script can be safely retried because its result
    # is output to a file that it would overwrite on a later
    # iteration.
    #
    # shellcheck disable=SC2154
    python buildscripts/evergreen_resmoke_job_count.py \
        --taskName "$task_name" \
        --buildVariant "$build_variant" \
        --distro "$distro_id" \
        --jobFactor "$resmoke_jobs_factor" \
        --jobsMax "$resmoke_jobs_max" \
        --outFile resmoke_jobs_expansion.yml

    exit_code=$?
    if [ $exit_code -eq 0 ]; then
        break
    fi

    if [ "$i" -eq 3 ]; then
        echo "Determining resmoke jobs failed 3 times."
        exit 1
    fi

    echo "Determining resmoke jobs failed, trying again..."
done
