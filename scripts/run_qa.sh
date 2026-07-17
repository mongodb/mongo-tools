#!/bin/bash

set -x
set -v
set -e

chmod +x bin/*
mv bin/* test/qa-tests/
cd test/qa-tests
chmod 400 jstests/libs/key*

PATH=/opt/mongodbtoolchain/v3/bin/:$PATH
python="${python_binary:-python3}"

$python -m venv venv
$python -m pip install pymongo==3.12.1 pyyaml
if [ "Windows_NT" = "$OS" ]; then
    $python -m pip install pywin32
fi
# shellcheck disable=SC2154 # resmoke_suite, resmoke_args, and excludes are Evergreen expansions
# shellcheck disable=SC2086 # resmoke_args is a space-separated list of extra flags that must be
# word-split, not a single value; quoting it would pass it (or, if empty, an empty string) as one
# argument instead of zero or more separate ones.
$python buildscripts/resmoke.py --suite="${resmoke_suite}" --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
