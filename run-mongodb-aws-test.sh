#!/bin/bash

set -o xtrace
set -o errexit  # Exit the script with error if any of the commands fail

############################################
#            Main Program                  #
############################################

# Supported/used environment variables:
#  MONGODB_URI    Set the URI, including an optional username/password to use
#                 to connect to the server via MONGODB-AWS authentication
#                 mechanism.

echo "Running MONGODB-AWS authentication tests"
# ensure no secrets are printed in log files
set +x

# make sure we're in the directory where the script lives

# COMMENT THIS OUT LATER!!!
echo bash source bracket 0: "${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR
OUTPUT_DIR="$SCRIPT_DIR/testing_output_aws_auth"
mkdir -p "$OUTPUT_DIR"

. ./set_goenv.sh
set_goenv || exit

# load the script
shopt -s expand_aliases # needed for `urlencode` alias
[ -s "$(pwd)/prepare_mongodb_aws.sh" ] && source "$(pwd)/prepare_mongodb_aws.sh"

MONGODB_URI=${MONGODB_URI:-"mongodb://localhost"}
MONGODB_URI="${MONGODB_URI}/aws?authMechanism=MONGODB-AWS"
if [[ -n ${SESSION_TOKEN} ]]; then
    MONGODB_URI="${MONGODB_URI}&authMechanismProperties=AWS_SESSION_TOKEN:${SESSION_TOKEN}"
fi

export MONGODB_URI="$MONGODB_URI"

# show test output
set -x

echo "Testing mongodump aws auth..."

if [ "$ON_EVERGREEN" = "true" ]; then
  (cd mongodump && go test > "$OUTPUT_DIR/$COMMON_SUBPKG.suite")
else
    (cd mongodump && go test $(buildflags) -ldflags "$(print_ldflags)" "$(print_tags $tags)" "$COVERAGE_ARGS" )
    exitcode=$?
fi

if [ -t /dev/stdin ]; then
    stty sane
fi

exit $ec

#go run "$(pwd)/mongo/testaws/main.go"
