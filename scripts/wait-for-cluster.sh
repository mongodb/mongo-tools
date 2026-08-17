#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${MONGOD_PORT:?}" "${MONGO_ARGS?}" "${MONGO_ARGS_TLS?}" "${USE_TLS?}"

REPLSET_NODES="${REPLSET_NODES:-1}"

if [ "$USE_TLS" = "true" ]; then
    MONGO_ARGS="$MONGO_ARGS_TLS"
fi
# shellcheck disable=SC2086 # $MONGO_ARGS intentionally word-split
./bin/mongo $MONGO_ARGS --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:$MONGOD_PORT\"); return true} catch(e){return false}}, \"timed out connection\")"

# Connectability is not enough on a set with more than one node: the tests write
# through this port, so wait until the node behind it has won the election.
if [ "$REPLSET_NODES" -gt 1 ]; then
    # shellcheck disable=SC2086 # $MONGO_ARGS intentionally word-split
    ./bin/mongo $MONGO_ARGS --nodb --eval "assert.soon(function(x){try{return new Mongo(\"localhost:$MONGOD_PORT\").getDB(\"admin\").isMaster().ismaster} catch(e){return false}}, \"timed out waiting for localhost:$MONGOD_PORT to become primary\")"
fi
