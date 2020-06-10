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

tags=""
if [ ! -z "$1" ]
  then
  	tags="$@"
fi
# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR
OUTPUT_DIR="$SCRIPT_DIR/testing_output_aws_auth"
mkdir -p "$OUTPUT_DIR"

. ./set_goenv.sh
set_goenv || exit

# remove stale packages
rm -rf vendor/pkg

# load the script
shopt -s expand_aliases # needed for `urlencode` alias
[ -s "$(pwd)/prepare_mongodb_aws.sh" ] && source "$(pwd)/prepare_mongodb_aws.sh"

MONGODB_URI=${MONGODB_URI:-"mongodb://localhost"}
MONGODB_URI="${MONGODB_URI}:33333/aws?authMechanism=MONGODB-AWS"
if [[ -n ${SESSION_TOKEN} ]]; then
    MONGODB_URI="${MONGODB_URI}&authMechanismProperties=AWS_SESSION_TOKEN:${SESSION_TOKEN}" # &ssl=true
fi

export MONGOD="$MONGODB_URI"
export TOOLS_TESTING_MONGOD="$MONGOD"
ec=0

# Run all tests depending on what flags are set in the environment
# TODO: mongotop needs a test
echo "the tags: " $tags
for i in mongodump mongorestore mongoimport mongoexport mongofiles ; do
        COMMON_SUBPKG=$(basename $i)
        COVERAGE_ARGS="";
        if [ "$RUN_COVERAGE" == "true" ]; then
          export COVERAGE_ARGS="-coverprofile=coverage_$COMMON_SUBPKG.out"
        fi
        if [ "$ON_EVERGREEN" = "true" ]; then
            (cd $i && go test $(buildflags) -ldflags "$(print_ldflags)" $tags -tags "$(print_tags $TOOLS_BUILD_TAGS)" "$COVERAGE_ARGS") # > "$OUTPUT_DIR/$COMMON_SUBPKG.suite"
            exitcode=$?
#            cat "$OUTPUT_DIR/$COMMON_SUBPKG.suite"
        else
            (cd $i && go test $(buildflags) -ldflags "$(print_ldflags)" "$(print_tags $tags)" "$COVERAGE_ARGS" )
            exitcode=$?
        fi
        if [ $exitcode -ne 0 ]; then
            echo "Error testing $i"
            ec=1
        fi
done

if [ -t /dev/stdin ]; then
    stty sane
fi

exit $ec
