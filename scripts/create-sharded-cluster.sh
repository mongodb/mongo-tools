#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${LOAD_LIBS_VERSION?}" "${MONGOD_PORT:?}"

echo "starting sharded cluster"
mkdir -p /data/db/
# use jsconfig.json to set baseUrl to find libs
mv test/shell_common/jsconfig.json ./
if [ -n "$LOAD_LIBS_VERSION" ]; then
    IMPORT_LOAD_LIBS="await import(\"../shell_common/libs/load_libs-${LOAD_LIBS_VERSION}.js\");"
fi
PATH=./bin:$PATH ./bin/mongo --port "$MONGOD_PORT" --nodb --eval "$IMPORT_LOAD_LIBS; var st = new ShardingTest({name: \"tools_sharded_cluster\", shards: 1, mongos: [{port: ${MONGOD_PORT}}], other: {rsOptions: {}, configOptions: {}}}); while(true){sleep(1000);}"
