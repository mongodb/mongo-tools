#!/bin/bash

set -o errexit
set -o pipefail

GOSEC_SARIF_REPORT=1 mise exec 'github:houseabsolute/precious' -- precious --quiet lint --all --command gosec

git diff --exit-code SARIF.json
