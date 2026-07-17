#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

: "${SLAVE?}" "${PASSWD?}" "${BUILD_VARIANT?}" "${BUILDER_NUM?}" "${TASK_NAME?}" "${EXECUTION?}"

cat >mci.buildlogger <<END_OF_CREDS
slavename='$SLAVE'
passwd='$PASSWD'
builder='MCI_$BUILD_VARIANT'
build_num=$BUILDER_NUM
build_phase='${TASK_NAME}_${EXECUTION}'
END_OF_CREDS
# Resmoke hardcodes the location of this file so we need to copy it to the working directory
# we run resmoke from.
cp mci.buildlogger test/qa-tests
