#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${LOAD_LIBS_VERSION?}" "${MONGOD_PORT:?}" "${MONGO_ARGS:?}" "${MONGO_ARGS_TLS?}" \
    "${REPLSETTEST_SSL_CONFIG?}" "${REPLSETTEST_TLS_CONFIG?}" "${USE_SSL?}" "${USE_TLS?}"

echo "starting repl set"
NODE_OPTIONS=""
mkdir -p /data/db/
if [ "$USE_TLS" = "true" ]; then
    NODE_OPTIONS="$REPLSETTEST_TLS_CONFIG"
    MONGO_ARGS="$MONGO_ARGS_TLS"
elif [ "$USE_SSL" = "true" ]; then
    NODE_OPTIONS="$REPLSETTEST_SSL_CONFIG"
fi
# use jsconfig.json to set baseUrl to find libs
mv test/shell_common/jsconfig.json ./
if [ -n "$LOAD_LIBS_VERSION" ]; then
    IMPORT_LOAD_LIBS="await import(\"../shell_common/libs/load_libs-${LOAD_LIBS_VERSION}.js\");"
fi
# shellcheck disable=SC2086 # $MONGO_ARGS intentionally word-split
PATH=./bin:$PATH ./bin/mongo $MONGO_ARGS --nodb --eval "$IMPORT_LOAD_LIBS; TestData = new Object(); TestData.minPort=\"${MONGOD_PORT}\"; var repl = new ReplSetTest({nodes:1, name:'repltester', nodeOptions: {$NODE_OPTIONS}});repl.startSet();repl.initiate();repl.awaitSecondaryNodes();while(true){sleep(1000);}"
