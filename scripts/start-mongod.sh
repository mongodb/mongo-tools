#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${MONGOD_PORT:?}" "${LOGFILE:?}"
: "${ADDITIONAL_ARGS?}" "${AWS_AUTH?}" "${EXTENSION?}" "${MONGOD_ARGS?}" "${MONGOD_ARGS_TLS?}" "${STORAGE_ENGINE?}" "${USE_TLS?}"

rm -rf mongodb/db_files "mongodb/$LOGFILE"
mkdir -p mongodb/db_files
if [ "$USE_TLS" = "true" ]; then
    MONGOD_ARGS="$MONGOD_ARGS_TLS"
elif [ "$AWS_AUTH" = "true" ]; then
    MONGOD_ARGS="--auth --setParameter authenticationMechanisms=MONGODB-AWS,SCRAM-SHA-256"
fi
echo "Starting mongod..."
storage_args=''
if [ "$STORAGE_ENGINE" != '' ]; then
    storage_args="--storageEngine $STORAGE_ENGINE"
fi
# shellcheck disable=SC2086 # $MONGOD_ARGS/$ADDITIONAL_ARGS/$storage_args are intentionally word-split
PATH=$PWD/bin:$PATH "./bin/mongod$EXTENSION" --port "$MONGOD_PORT" $MONGOD_ARGS $ADDITIONAL_ARGS --dbpath mongodb/db_files --setParameter=enableTestCommands=1 $storage_args
