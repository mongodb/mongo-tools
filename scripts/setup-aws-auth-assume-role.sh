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

# The aws_e2e_assume_role script requires python3 with boto3.
# Set up or use an existing python virtualenv for boto3.
venv='venv'
venvNonWindows() {
    # shellcheck disable=SC1091 # the venv activate script is created at runtime, not in the repo
    . "$venv"/bin/activate
    "$python" -m pip install boto3
}
venvWindows() {
    # shellcheck disable=SC1091 # the venv activate script is created at runtime, not in the repo
    . "$venv"/Scripts/activate
    pip install boto3
}

if [ -f "$venv"/bin/activate ]; then
    echo 'activating existing virtualenv'
    venvNonWindows
elif [ -f "$venv"/Scripts/activate ]; then
    echo 'activating existing virtualenv'
    dos2unix "$venv"/Scripts/activate
    venvWindows
elif "$python" -m venv "$venv"; then
    echo 'creating new virtualenv'
    if [ -f "$venv"/bin/activate ]; then
        echo 'activating new virtualenv'
        venvNonWindows
    elif [ -f "$venv"/Scripts/activate ]; then
        echo 'activating new virtualenv'
        dos2unix "$venv"/Scripts/activate
        venvWindows
    fi
fi

"$python" -m pip list

../../../bin/mongo --nodb aws_e2e_assume_role.js
