#!/bin/bash

set -x
set -v
set -e

export PATH="${EVG_WORKDIR:?}/.local/bin:$PATH"
export MISE_DATA_DIR="${EVG_WORKDIR:?}/.local/share/mise"

chmod +x bin/*
mv bin/* test/qa-tests/
cd test/qa-tests
chmod 400 jstests/libs/key*

mise exec python -- python3 -m venv venv
# shellcheck disable=SC1091 # this will exist in CI when we run this script
. venv/bin/activate
pip install -r ../../requirements.txt

# shellcheck disable=SC2154 # resmoke_suite, resmoke_args, and excludes are Evergreen expansions
# shellcheck disable=SC2086 # resmoke_args is a space-separated list of extra flags that must be
# word-split, not a single value; quoting it would pass it (or, if empty, an empty string) as one
# argument instead of zero or more separate ones.
# `mise exec python --` alone would re-prepend mise's own python bin dir onto PATH ahead of the
# activated venv, so a bare `python3` would resolve to mise's interpreter instead of the venv's,
# missing the packages we just pip installed. Pointing it at the venv's own python3 directly
# avoids that, while still going through mise exec so the pinned python tool stays installed.
mise exec python -- "$PWD/venv/bin/python3" buildscripts/resmoke.py --suite=${resmoke_suite} --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
