#!/bin/bash

# This script intentionally doesn't set errexit or pipefail, since these commands can fail and that's ok.
set -o verbose

: "${KILLALL_MCI?}"

# don't attempt to abort on any distro which has a special way of
# killing everything (i.e. using taskkill on Windows)
if [ "$KILLALL_MCI" = "" ]; then
    all_tools="bsondump mongodump mongoexport mongofiles mongoimport mongorestore mongostat mongotop"
    # send SIGABRT to print a stacktrace for any hung tool
    pkill -ABRT "^($(echo -n "$all_tools" | tr ' ' '|'))\$"
    # git the processes a second or two to dump their stacks
    sleep 10
fi
