#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${MONGOD_PORT:?}" "${MONGO_ARGS?}" "${MONGO_ARGS_TLS?}" "${USE_TLS?}"

if [ "$USE_TLS" = "true" ]; then
    MONGO_ARGS="$MONGO_ARGS_TLS"
fi
# shellcheck disable=SC2086 # $MONGO_ARGS intentionally word-split
./bin/mongo $MONGO_ARGS --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:$MONGOD_PORT\"); return true} catch(e){return false}}, \"timed out connection\")"
