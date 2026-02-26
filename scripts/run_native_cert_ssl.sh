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

mise exec python -- python3 -m venv venv
. venv/bin/activate
pip install -r ../../requirements.txt
python3 buildscripts/resmoke.py --suite=native_cert_ssl  --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
