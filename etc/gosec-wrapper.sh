#!/usr/bin/env bash

set -o errexit
set -o pipefail

if [ -n "$GOSEC_SARIF_REPORT" ]; then
    mise exec 'github:securego/gosec' -- gosec -fmt sarif -track-suppressions $@ | tee SARIF.json
else
    mise exec 'github:securego/gosec' -- gosec $@
fi
