#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"

go run build.go -v build
# Copy mongodump and mongorestore to mongosync/dist
mkdir -p "$EVG_WORKDIR/src/mongosync/dist"
cp -p bin/mongodump bin/mongorestore "$EVG_WORKDIR/src/mongosync/dist"
