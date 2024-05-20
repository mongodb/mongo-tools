#!/usr/bin/env bash

set -o errexit
set -o pipefail

# This rule complains about reading or writing to paths based on user input,
# but most of the tools exist for the purpose of reading and writing from/to
# user-provided paths.
EXCLUDE="-exclude=G304"
SEVERITY="-severity=high"

if [ -n "$GOSEC_SARIF_REPORT" ]; then
    gosec -fmt sarif $EXCLUDE $SEVERITY -track-suppressions $@ | tee SARIF.json
else
    gosec $EXCLUDE $SEVERITY $@
fi
