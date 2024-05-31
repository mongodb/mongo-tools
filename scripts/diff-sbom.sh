#!/bin/bash

set -e
set -x

if [ -z "$1" ]; then
    echo "You must provide a path to an SBOM to diff"
    exit 1
fi

# The silkbomb tool updates the timestamp and version field for the doc on
# every run, even if nothing else changed.
git diff --exit-code "$1"
