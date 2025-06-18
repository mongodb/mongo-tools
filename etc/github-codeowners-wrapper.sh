#!/usr/bin/env bash

set -o errexit
set -o pipefail

STATUS=0

OUTPUT=$(./node_modules/.bin/github-codeowners audit --unloved --only-git)

if [ -n "$OUTPUT" ]; then
    echo "The repo contains unowned files. Please update the '.github/CODEOWNERS' file to include these files:"
    echo ""
    echo "$OUTPUT"
    echo ""
    STATUS=1
fi

OUTPUT=$(./node_modules/.bin/github-codeowners validate 2>&1)
if [ -n "$OUTPUT" ]; then
    echo "The '.github/CODEOWNERS' file is not valid:"
    echo ""
    echo "$OUTPUT"
    echo ""
    STATUS=1
fi

exit $STATUS
