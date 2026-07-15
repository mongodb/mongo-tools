#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

: "${TEST_PATH:?}" "${LOAD_LIBS_VERSION?}"

chmod +x bin/*
mv bin/* "$TEST_PATH"/
cd "$TEST_PATH"
if [ -n "$LOAD_LIBS_VERSION" ]; then
    IMPORT_LOAD_LIBS="await import(\"../shell_common/libs/load_libs-${LOAD_LIBS_VERSION}.js\");"
fi
./mongo --nodb --eval "$IMPORT_LOAD_LIBS" lib/run_mongod.js jstests/tool/*.js
