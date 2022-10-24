#!/bin/bash

set -x
set -v
set -e

echo "starting sharded cluster"

if [ "${USE_TLS}" = "true" ]; then
  MLAUNCH_ARGS="$MLAUNCH_ARGS_TLS"
fi

CLUSTER_TYPE="--replicaset --nodes 3"
if [ -n "$MLAUNCH_SHARDED_CLUSTER" ]; then
    CLUSTER_TYPE="$CLUSTER_TYPE --sharded 3"
fi

# The ./bin directory contains our downloaded mongod and mongos.
PATH=./bin:$PATH

mlaunch $CLUSTER_TYPE $MLAUNCH_ARGS
