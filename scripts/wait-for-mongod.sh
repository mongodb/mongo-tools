#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${MONGO_ARGS:?}"
: "${AWS_AUTH?}" "${EXTENSION?}" "${MONGO_ARGS_TLS?}" "${USE_TLS?}"

SECS=0
if [ "$USE_TLS" = "true" ]; then
    MONGO_ARGS="$MONGO_ARGS_TLS"
elif [ "$AWS_AUTH" = "true" ]; then
    MONGO_ARGS="--port 33333"
fi
while true; do
    set +o errexit
    # shellcheck disable=SC2086 # $MONGO_ARGS is intentionally word-split
    "./bin/mongo${EXTENSION}" $MONGO_ARGS --eval 'true;'
    status=$?
    # This overwrites "$?".
    set -o errexit

    if [ "$status" = "0" ]; then
        echo "mongod ready"
        exit 0
    else
        SECS=$((SECS + 1))
        if [ "$SECS" -gt 100 ]; then
            echo "mongod not ready after 100 seconds"
            exit 1
        fi
        echo "waiting for mongod $MONGO_ARGS to be ready..."
        sleep 1
    fi
done
