#!/bin/bash

set -o errexit
set -o verbose

echo "The current pwd: $(pwd)"
cd src/resmoke

# The 'username' and 'passwd' are nonsense. It doesn't matter what they are actually set to. The
# 'builder' and 'build_num' are important because they tell logkeeper where to append information
# to. The 'build_phase' isn't vital to logkeeper.
#
# shellcheck disable=SC2154
cat >mci.buildlogger <<END_OF_CREDS
slavename='nonsense_username'
passwd='nonsense_pwd'
builder='MCI_${build_variant}'
build_num=${builder_num}
build_phase='${task_name}_${execution}'
END_OF_CREDS
