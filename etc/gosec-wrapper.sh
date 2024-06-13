#!/usr/bin/env bash

set -o errexit
set -o pipefail

if [ -n "$GOSEC_SARIF_REPORT" ]; then
    ./dev-bin/gosec -fmt sarif -track-suppressions $@ | tee SARIF.json
else
    ./dev-bin/gosec $@
fi
