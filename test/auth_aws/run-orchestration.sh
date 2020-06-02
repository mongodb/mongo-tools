#!/bin/sh
set -o xtrace   # Write all commands first to stderr
set -o errexit  # Exit the script with error if any of the commands fail


AUTH=${AUTH:-noauth}
SSL=${SSL:-nossl}
TOPOLOGY=${TOPOLOGY:-server}
STORAGE_ENGINE=${STORAGE_ENGINE}
# Set to a non-empty string to use the <topology>/disableTestCommands.json
# cluster config, eg DISABLE_TEST_COMMANDS=1
DISABLE_TEST_COMMANDS=${DISABLE_TEST_COMMANDS}
MONGODB_VERSION=${MONGODB_VERSION:-latest}

MO_START=$(date +%s)

ORCHESTRATION_FILE=${ORCHESTRATION_FILE}

export ORCHESTRATION_FILE="configs/servers/auth-aws.json"
export ORCHESTRATION_URL="http://localhost:8889/v1/servers"

# Start mongo-orchestration
sh start-orchestration.sh "orchestration"

pwd
if ! curl --silent --show-error --data @"$ORCHESTRATION_FILE" "$ORCHESTRATION_URL" --max-time 600 --fail -o tmp.json; then
  echo Failed to start cluster, see orchestration/out.log:
  cat orchestration/out.log
  echo Failed to start cluster, see orchestration/server.log:
  cat orchestration/server.log
  exit 1
fi
cat tmp.json
URI=$(python -c 'import sys, json; j=json.load(open("tmp.json")); print(j["mongodb_auth_uri" if "mongodb_auth_uri" in j else "mongodb_uri"])' | tr -d '\r')
echo 'MONGODB_URI: "'$URI'"' > mo-expansion.yml
echo "Cluster URI: $URI"

MO_END=$(date +%s)
MO_ELAPSED=$(expr $MO_END - $MO_START)
DL_ELAPSED=$(expr $DL_END - $DL_START)
cat <<EOT >> $DRIVERS_TOOLS/results.json
{"results": [
  {
    "status": "PASS",
    "test_file": "Orchestration",
    "start": $MO_START,
    "end": $MO_END,
    "elapsed": $MO_ELAPSED
  },
]}

EOT
