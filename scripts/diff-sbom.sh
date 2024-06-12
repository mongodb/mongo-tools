#!/bin/bash

set -e
set -x

if [ -z "$1" ]; then
    echo "You must provide a path to an SBOM to diff"
    exit 1
fi

# The silkbomb tool updates the timestamp and version field for the doc on
# every run, even if nothing else changed.
#
# TODO (TOOLS-3561): Remove the `--ignore-matching-lines` bit.
git diff --ignore-matching-lines '"timestamp":\s+".+"|"version":\s+[0-9]+' --exit-code "$1"
