#!/bin/bash

set -x
set -v
set -e

chmod +x bin/*
mv bin/* test/qa-tests/
cd test/qa-tests
chmod 400 jstests/libs/key*

PATH=/opt/mongodbtoolchain/v3/bin/:$PATH
python="python3"
if [ "Windows_NT" = "$OS" ]; then
    python="py.exe -3"
fi
$python -m venv venv
pip3 install pymongo==3.12.1 pyyaml
if [ "Windows_NT" = "$OS" ]; then
    pip3 install pywin32
fi
$python buildscripts/resmoke.py --suite=${resmoke_suite} --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
