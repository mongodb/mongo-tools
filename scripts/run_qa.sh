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
$python buildscripts/resmoke.py --suite=${resmoke_suite} --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
