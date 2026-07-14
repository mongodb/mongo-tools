#!/bin/bash

set -x
set -v
set -e

chmod +x bin/*
mv bin/* test/qa-tests/
cd test/qa-tests
chmod 400 jstests/libs/key*

if [[ $OSTYPE == "darwin"* ]]; then
    keychain="/Users/$(whoami)/Library/Keychains/login.keychain-db"
    sudo security add-trusted-cert -d -k "$keychain" jstests/libs/trusted-ca.pem
    sudo security add-trusted-cert -d -k /Library/Keychains/System.keychain jstests/libs/trusted-ca.pem
fi

PATH=/opt/mongodbtoolchain/v3/bin/:$PATH
python="${python_binary:-python3}"

$python -m venv venv
$python -m pip install pymongo==3.12.1 pyyaml
if [ "Windows_NT" = "$OS" ]; then
    $python -m pip install pywin32
fi
# shellcheck disable=SC2154 # resmoke_args and excludes are Evergreen expansions
# shellcheck disable=SC2086 # resmoke_args is a space-separated list of extra flags that must be
# word-split, not a single value; quoting it would pass it (or, if empty, an empty string) as one
# argument instead of zero or more separate ones.
$python buildscripts/resmoke.py --suite=native_cert_ssl --continueOnFailure --log=buildlogger --reportFile=../../report.json ${resmoke_args} --excludeWithAnyTags="${excludes}"
