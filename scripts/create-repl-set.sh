#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${LOAD_LIBS_VERSION?}" "${MONGOD_PORT:?}" "${MONGO_ARGS:?}" "${MONGO_ARGS_TLS?}" \
    "${REPLSETTEST_SSL_CONFIG?}" "${REPLSETTEST_TLS_CONFIG?}" "${USE_SSL?}" "${USE_TLS?}"

REPLSET_NODES="${REPLSET_NODES:-1}"

# The tests all connect to MONGOD_PORT, which is the first node of the set, so
# that node has to be the primary. Give it a higher priority than the rest, which
# makes it win the election rather than leaving that to chance.
INITIATE="repl.initiate()"
if [ "$REPLSET_NODES" -gt 1 ]; then
    INITIATE="var cfg = repl.getReplSetConfig(); cfg.members[0].priority = 2; repl.initiate(cfg)"
fi

echo "starting repl set"
# ReplSetTest passes enableTestCommands to the nodes only when the shell itself was
# started with it, and that default lives in the shell binary rather than in this
# repo. Tests that drive a server failpoint need it, so set it explicitly.
NODE_OPTIONS='setParameter: {enableTestCommands: 1}'
mkdir -p /data/db/
if [ "$USE_TLS" = "true" ]; then
    NODE_OPTIONS="$NODE_OPTIONS, $REPLSETTEST_TLS_CONFIG"
    MONGO_ARGS="$MONGO_ARGS_TLS"
elif [ "$USE_SSL" = "true" ]; then
    NODE_OPTIONS="$NODE_OPTIONS, $REPLSETTEST_SSL_CONFIG"
fi
# use jsconfig.json to set baseUrl to find libs
mv test/shell_common/jsconfig.json ./
if [ -n "$LOAD_LIBS_VERSION" ]; then
    IMPORT_LOAD_LIBS="await import(\"../shell_common/libs/load_libs-${LOAD_LIBS_VERSION}.js\");"
fi
# shellcheck disable=SC2086 # $MONGO_ARGS intentionally word-split
PATH=./bin:$PATH ./bin/mongo $MONGO_ARGS --nodb --eval "$IMPORT_LOAD_LIBS; TestData = new Object(); TestData.minPort=\"${MONGOD_PORT}\"; var repl = new ReplSetTest({nodes:${REPLSET_NODES}, name:'repltester', nodeOptions: {$NODE_OPTIONS}});repl.startSet();${INITIATE};repl.awaitSecondaryNodes();while(true){sleep(1000);}"
