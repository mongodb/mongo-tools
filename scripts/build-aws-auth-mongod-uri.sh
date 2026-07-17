#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

# This function's commands originally ran with working_dir set to
# common/testdata/lib; we now run from the module root (so ./scripts/ resolves)
# and cd into that directory to preserve the body's relative-path behavior.
cd common/testdata/lib

PATH=/opt/mongodbtoolchain/v3/bin/:$PATH
python="python3"
if [ "Windows_NT" = "$OS" ]; then
    python="/cygdrive/c/python/Python311/python"
fi

jsonkey() { "$python" -c "import sys, json; sys.stdout.write(json.load(sys.stdin)['$1'])" <creds.json; }
urlencode() { "$python" -c "import sys, urllib.parse as ul; sys.stdout.write(ul.quote_plus('$1'))"; }

USER=$(jsonkey AccessKeyId)
USER=$(urlencode "$USER")

PASS=$(jsonkey SecretAccessKey)
PASS=$(urlencode "$PASS")

MONGOD_URI="mongodb://$USER:$PASS@localhost:33333/?authMechanism=MONGODB-AWS"

SESSION_TOKEN=$(jsonkey SessionToken)
SESSION_TOKEN=$(urlencode "$SESSION_TOKEN")
# The original test for a session token is deliberately left unquoted: `[ -n $X ]`
# with an unquoted empty value tests "-n" itself (always true), and quoting it
# would change that behavior. shellcheck disabled to preserve the original logic.
# shellcheck disable=SC2070,SC2086
if [ -n $SESSION_TOKEN ]; then
    MONGOD_URI="$MONGOD_URI&authMechanismProperties=AWS_SESSION_TOKEN:$SESSION_TOKEN"
fi

echo -n "$MONGOD_URI" >MONGOD_URI
