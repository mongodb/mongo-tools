#!/bin/bash

set -o errexit
#set -o verbose
set -x

resmoke_dir="$(pwd)/$resmoke_dir"

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=etc/find-recent-python.sh
. "$SCRIPT_DIR/find-recent-python.sh"

python3 -m venv venv

# shellcheck disable=SC1091
source venv/bin/activate
python3 -m pip --disable-pip-version-check install "db-contrib-tool==1.1.5"

# The pinned version of Resmoke that we want to download.
resmoke_version="${pinned_resmoke_version:?}"
mongo_shell_server_version="${resmoke_version}"
echo "Resmoke version and mongo shell to download: $resmoke_version"

source_server_version="${src_pinned_server_version:?}"
echo "Server compiled binaries version to download for source: $source_server_version"

destination_server_version="${dst_pinned_server_version:?}"
echo "Server compiled binaries version to download for destination: $destination_server_version"

db_contrib_tool_server_version=$destination_server_version
mongo_shell_server_version=$destination_server_version

# The patch build version overrides other specified versions.
# This is passed in via the Evergreen CLI.
if [[ -n $src_server_version_for_patch ]]; then
    source_server_version="$src_server_version_for_patch"
    echo "Source patch server version exists: $src_server_version_for_patch"
fi
if [[ -n $dst_server_version_for_patch ]]; then
    destination_server_version="$dst_server_version_for_patch"
    echo "Destination patch server version exists: $dst_server_version_for_patch"
fi

# We invoke db-contrib-tool to download server artifacts. Invocations of db-contrib-tool download
# Resmoke and compiled binaries into the repro_envs/ folder. Each invocation of db-contrib-tool
# creates a folder within repro_envs/, with the folder name being the name of the Evergreen patch from
# which everything was downloaded.
#
# In the case in which we want the newest commit on a branch to be downloaded, db-contrib-tool
# will query Evergreen for the latest waterfall patch available for that branch, and then will
# create a folder with the name of the latest patch that has all the artifacts within it. For example,
# on providing server version "6.0" to db-contrib-tool, we might see a folder called "mongodb_mongo_v6.0_<commit hash>"
# in repro_envs/.
#
# Similarly, when we want a particular Evergreen version - provided through a patch ID or a pinned
# waterfall nightly run - db-contrib-tool will create a folder with the name of the patch ID or pinned
# waterfall patch.
#
# The db-contrib-tool code first tries to get an Evergreen release artifact, but will fall back to
# an actual release tarball. These two things have different directory structure, and the actual
# release tarballs do not contain all the tools we need.
#
# The directory structure looks like this after calling db-contrib-tool to download some patch
# "mongodb_mongo_v6.0_<commit hash>":
#    repro_envs
#        +- mongodb_mongo_v6.0_<commit hash>
#              +- <commit hash>
#                    +- buildscripts (folder with Resmoke and friends)
#                    +- dist-test (folder with compiled binaries)
#                    +- etc (folder with pip requirements, etc.)

# This command just downloads resmoke code, which is Python, so it doesn't need to match the
# platform/arch we're running on.
#
# Here the pin is at "$resmoke_version", so this will download artifacts into
# "repro_envs/$resmoke_version".
db-contrib-tool setup-repro-env \
    --edition enterprise \
    --downloadArtifacts \
    --skipBinaries \
    --platform rhel8 \
    --architecture x86_64 \
    --installDir "repro_envs/$resmoke_version" \
    "$db_contrib_tool_server_version"
# TODO: shyam - confirm whether the above is right.
# Separately, the mongosync Evergreen task structure expects server items, both resmoke and compiled
# binaries, to live  within "$resmoke_dir" (this translates to "resmoke_src/resmoke" at the time of writing
# this comment, but that "$resmoke_dir" will be translated to "resmoke_src/resmoke" is something we cannot
# depend on).
# Further, the succeeding Evergreen tasks need to know exactly where the artifacts (buildscripts/,
# dist-test/, etc/, etc.) are. Leaving the artifacts in a folder whose name changes based on the patch
# is not allowed. Therefore, we need to move the artifacts into a deterministically named folder
# called "${src_mongo_version}_${dst_mongo_version}", which is set in evergreen.yml. This will let subsequent
# tasks know where to find the artifacts. Server binaries are stored in
# So finally, for mongosync Evergreen tasks to function, the tasks would like this directory structure:
#    "resmoke_dir"
#        +- "${src_mongo_version}_${dst_mongo_version}"
#              +- buildscripts
#              +- etc
#              +- evergreen
#              +- jstests
#              +- src
#              +- ${src_mongo_version}
#                    +- bin
#              +- ${dst_mongo_version}
#                    +- bin

mkdir -p "$resmoke_dir/$resmoke_version"
# Move contents and hidden yml files from the downloaded commit hash directory to "$resmoke_dir/$resmoke_version".
mv -v "repro_envs/$resmoke_version"/*/{*,.*.yml} "$resmoke_dir/$resmoke_version"

# Change the name of the folder (currently the patch name) to "${src_mongo_version}_${dst_mongo_version}" instead.
#
versions_dir="${src_mongo_version:?}_${dst_mongo_version:?}"
mv "$resmoke_dir/$resmoke_version" "$resmoke_dir/$versions_dir"

# Symlinks to the binaries will be created in link_dir. When running locally,
# instead of on evergreen, we want to be able to set the link_dir so we keep the top-level directory clean.
link_dir=$(pwd)

if [[ -n $binary_link_dir ]]; then
    link_dir="$binary_link_dir"
    echo "Link directory: $binary_link_dir"
fi

function downloadBinaries() {
    version=$1
    version_dir=$2

    # Unlike download-mongodb.sh, this script expects to see these as their expansion names, because
    # it's executed with `add_expansions_to_env: true`.
    PLATFORM_CMD=""
    if [ -n "${server_platform}" ]; then
        PLATFORM_CMD="--platform ${server_platform}"
    fi

    ARCHITECTURE_CMD=""
    if [ -n "${server_architecture}" ]; then
        ARCHITECTURE_CMD="--architecture ${server_architecture}"
    fi

    # We intentionally don't quote the *_CMD vars, since they contain a flag and a flag value. If we
    # quote them, then `db-contrib-tool` sees an argument like `--platform amazon2` as one string,
    # rather than `--platform` followed by `amazon2`.
    #
    # shellcheck disable=SC2086
    db-contrib-tool setup-repro-env \
        --edition enterprise \
        ${PLATFORM_CMD} \
        ${ARCHITECTURE_CMD} \
        --installDir "repro_envs/$version_dir" \
        --linkDir "$link_dir" \
        "$version"

    # When running this code locally the version dir has a dot in it, which
    # breaks running resmoke because it expects the dir to be something like
    # "v60". I'm not sure why this works okay in CI.
    version_dir="${version_dir/./}"
    mkdir "$resmoke_dir/$versions_dir/$version_dir"

    # This is the directory structure for Evergreen artifacts.
    disttest=$(find "repro_envs/$version_dir" -type d -name dist-test)
    if [ -n "$disttest" ]; then
        cp -R repro_envs/"$version_dir"/*/dist-test/* "$resmoke_dir/$versions_dir/$version_dir"
    else
        cp -R repro_envs/"$version_dir"/*/mongodb-*/* "$resmoke_dir/$versions_dir/$version_dir"
    fi

    rm -r "repro_envs/$version_dir"
}

downloadBinaries "$source_server_version" "$src_mongo_version"
if [ "$source_server_version" != "$destination_server_version" ]; then
    downloadBinaries "$destination_server_version" "$dst_mongo_version"
fi

if [ "$mongo_shell_server_version" == "$source_server_version" ]; then
    cp -R "$resmoke_dir/$versions_dir/$src_mongo_version" "$resmoke_dir/$versions_dir/mongo-for-shell"
elif [ "$mongo_shell_server_version" == "$destination_server_version" ]; then
    cp -R "$resmoke_dir/$versions_dir/$dst_mongo_version" "$resmoke_dir/$versions_dir/mongo-for-shell"
else
    downloadBinaries "$mongo_shell_server_version" mongo-for-shell
fi

echo "RESULT OF RESMOKE AND BINARIES DOWNLOAD"
echo $(pwd)
ls -lah