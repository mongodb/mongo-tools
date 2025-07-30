#!/bin/bash

set -o errexit
set -o pipefail

GOSEC_SARIF_REPORT=1 ./dev-bin/precious --quiet lint --all --command gosec

git diff --exit-code SARIF.json
