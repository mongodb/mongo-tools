#!/bin/bash

set -x
set -v
set -e

chmod +x bin/*
mv bin/* test/qa-tests/
cd test/qa-tests
chmod 400 jstests/libs/key*

if [[ "$OSTYPE" == "darwin"* ]]; then
    keychain="/Users/$(whoami)/Library/Keychains/login.keychain-db"
    sudo security add-trusted-cert -d -k $keychain jstests/libs/trusted-ca.pem
    sudo security add-trusted-cert -d -k /Library/Keychains/System.keychain jstests/libs/trusted-ca.pem
fi

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
$python buildscripts/resmoke.py --suite=native_cert_ssl  --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
