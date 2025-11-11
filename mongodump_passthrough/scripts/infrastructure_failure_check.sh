#!/bin/bash

set -o verbose

# shellcheck disable=SC2154
cd "$resmoke_dir" || exit 1

if [ -f infrastructure_failure ]; then
    exit "$(cat infrastructure_failure)"
fi
