#!/bin/bash

set -o errexit
set -o verbose

mkdir -p wheelhouse

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=etc/functions.sh
source "$SCRIPT_DIR/../../etc/functions.sh"

# shellcheck source=etc/find-recent-python.sh
. "$SCRIPT_DIR/../../etc/find-recent-python.sh"

echo "Installing pip ..."
python3 -m pip --disable-pip-version-check install "pip==21.0.1" "wheel==0.37.0" || exit 1

mongo_base="${resmoke_dir:?}/${src_mongo_version:?}_${dst_mongo_version:?}"

legacy_reqs_file="$mongo_base/etc/pip/dev-requirements.txt"

if [ -e "$mongo_base/poetry.lock" ]; then
    echo Installing Poetry ...
    python3 -m pip --disable-pip-version-check install poetry poetry-plugin-export

    curdir="$(pwd)"
    pushd "$mongo_base"

    reqs_file_path="$curdir/poetry-requirements.txt"

    echo Generating a requirements file from Poetry ...
    if ! export_server_poetry_requirements python3 >"$reqs_file_path"; then
        echo "Failed to export Poetry requirements"
        exit 1
    fi

    echo "Wrote requirements to $reqs_file_path"

    popd
elif [ -e "$legacy_reqs_file" ]; then
    echo "Using mongo repo’s requirements file ..."
    reqs_file_path="$legacy_reqs_file"
else
    echo "Found neither Poetry nor $legacy_reqs_file .. what gives??"
    exit 1
fi

if ! python3 -m pip \
    --disable-pip-version-check \
    download \
    --dest=wheelhouse \
    --log install.log \
    --requirement "${reqs_file_path:?}"; then
    echo "Pip download error"
    cat install.log
    exit 1
fi
