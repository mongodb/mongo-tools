#!/bin/bash

set -o errexit
set -o pipefail

# verbose is intentionally disabled since this runs every time someone enters the project directory.

cd "$MISE_PROJECT_ROOT"

command -v eslint >/dev/null && command -v github-codeowners >/dev/null && command -v oxfmt >/dev/null && exit 0

set -o verbose

npm install
