#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

: "${AUTH_PASSWORD:?}" "${AUTH_USERNAME:?}" "${MONGO_ARGS:?}"
: "${EXTENSION?}" "${MONGO_ARGS_TLS?}" "${USE_TLS?}"

if [ "$USE_TLS" = "true" ]; then
    MONGO_ARGS="$MONGO_ARGS_TLS"
fi
# shellcheck disable=SC2086 # $MONGO_ARGS is intentionally word-split
echo "db.createUser({ user: '$AUTH_USERNAME', pwd: '$AUTH_PASSWORD', roles: [{ role: '__system', db: 'admin' }] });" | ./bin/mongo$EXTENSION $MONGO_ARGS admin
