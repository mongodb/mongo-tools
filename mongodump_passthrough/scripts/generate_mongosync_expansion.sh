#!/bin/bash

set -o errexit
set -o xtrace

# shellcheck disable=SC2154
cd "$mongosync_dir" || exit 1

export MONGOSYNC_BUILD_TAGS="$BUILD_TAGS"

# We get the raw version string (r1.2.3-45-gabcdef) from git.
MONGOSYNC_VERSION="$(git describe)"
export MONGOSYNC_VERSION
# If this is a patch build, we add the patch version id to the version string so we know
# this build was a patch, and which evergreen task it came from.
if [ "$IS_PATCH" = "true" ]; then
    export MONGOSYNC_VERSION="$MONGOSYNC_VERSION-patch-$VERSION_ID"
fi

# shellcheck disable=SC2154
cat <<EOT >mongosync_expansion.yml
MONGOSYNC_VERSION: "$MONGOSYNC_VERSION"
PREPARE_SHELL: |
  set -o errexit
  set -o xtrace

  # We add $HOME to $PATH because the evergreen client binary lives at ~/evergreen.
  export PATH="${workdir}/.local/bin:$HOME:$PATH"

  # Set OS-level default Go configuration
  UNAME_S=$(PATH="/usr/bin:/bin" uname -s)
  # Set OS-level compilation flags
  if [ "$UNAME_S" = Darwin ]; then
    export CGO_CFLAGS="-mmacosx-version-min=10.15"
    export CGO_LDFLAGS="-mmacosx-version-min=10.15"
  fi

  export MISE_DATA_DIR="${workdir}/.local/share/mise"

  export MONGOSYNC_DEBUG_MAGEFILE=1
  export MAGEFILE_CACHE="${workdir}/.mage"

  export MONGOSYNC_BUILD_TAGS="$MONGOSYNC_BUILD_TAGS"
  export MONGOSYNC_VERSION="$MONGOSYNC_VERSION"

  set +o xtrace
  export AWS_ACCESS_KEY_ID='${release_aws_access_key_id}'
  export AWS_SECRET_ACCESS_KEY='${release_aws_secret}'
  set -o xtrace

  export EVG_WORKDIR='${workdir}'
  export EVG_IS_PATCH='${is_patch}'
  export EVG_IS_COMMIT_QUEUE='${is_commit_queue}'
  export EVG_TRIGGERED_BY_TAG='${triggered_by_git_tag}'
  export EVG_BRANCH='${branch_name}'
  if [ -n "${fake_tag_for_release_testing}" ]; then
    export EVG_TRIGGERED_BY_TAG="${fake_tag_for_release_testing}"
    export IS_FAKE_TAG=1
  fi

  # We tag the binary when it will be given to customers. We should only set the Segment write key
  # for non-testing use cases.
  set +o xtrace
  if [ '${triggered_by_git_tag}' != '' ]; then
    export SEGMENT_WRITE_KEY='${segment_write_key_prod}'
  fi;
  set -o xtrace

  export EVG_BUILD_ID='${build_id}'
  export EVG_TASK_ID='${task_id}'
  export EVG_TASK_NAME='${task_name}'
  export EVG_VERSION='${version_id}'
  export EVG_VARIANT='${build_variant}'
  if [ '${_platform}' != '' ]; then
    export EVG_VARIANT='${_platform}'
  fi
  export EVG_GITHUB_COMMIT='${github_commit}'
  export EVG_USER='${evg_user}'
  set +o xtrace
  export EVG_KEY='${evg_key}'
  set -o xtrace
  export SERVER_PLATFORM='${server_platform}'
  export SERVER_ARCHITECTURE='${server_architecture}'

  set +o xtrace
  export MONGOSYNC_YCSB_ATLAS_PUBLIC_API_KEY='${ycsb_atlas_public_api_key}'
  export MONGOSYNC_YCSB_ATLAS_PRIVATE_API_KEY='${ycsb_atlas_private_api_key}'
  set -o xtrace
  export MONGOSYNC_YCSB_COUNTER='${MONGOSYNC_YCSB_COUNTER}'
EOT
# See what we've done
cat mongosync_expansion.yml
