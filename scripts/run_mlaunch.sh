#!/bin/bash

set -x
set -v
set -e

echo "starting sharded cluster"

MLAUNCH_ARGS="--port 33333"

if [ "$USE_SSL" = "true" ]; then
    MLAUNCH_ARGS="$MLAUNCH_ARGS --sslMode requireSSL --sslCAFile common/db/testdata/ca-ia.pem --sslPEMKeyFile common/db/testdata/test-server.pem --sslClientCertificate common/db/testdata/test-client.pem --sslAllowInvalidCertificates"
fi

if [ "$USE_TLS" = "true" ]; then
    MLAUNCH_ARGS="$MLAUNCH_ARGS --tlsMode requireTLS --tlsCAFile common/db/testdata/ca-ia.pem --tlsCertificateKeyFile common/db/testdata/test-server.pem --tlsClientCertificateKeyFile common/db/testdata/test-client.pem --tlsAllowInvalidCertificates"
fi

if [ "$AWS_AUTH" = "true" ]; then
    MLAUNCH_ARGS="$MLAUNCH_ARGS --auth --setParameter authenticationMechanisms=MONGODB-AWS,SCRAM-SHA-256"
fi

if [ "$USE_AUTH" = "true" ]; then
    MLAUNCH_ARGS="$MLAUNCH_ARGS --auth"
fi

if [ -n "$STORAGE_ENGINE" ]; then
    MLAUNCH_ARGS="$MLAUNCH_ARGS --storageEngine $STORAGE_ENGINE"
fi

CLUSTER_TYPE="--replicaset --nodes 3"
if [ -n "$MLAUNCH_SHARDED_CLUSTER" ]; then
    CLUSTER_TYPE="$CLUSTER_TYPE --sharded 3"
elif [ -n "$MLAUNCH_SINGLE_NODE" ]; then
    CLUSTER_TYPE="--single"
fi

# The ./bin directory contains our downloaded mongod and mongos.
PATH=./bin:$HOME/.local/bin:$PATH

mlaunch $CLUSTER_TYPE $MLAUNCH_ARGS
