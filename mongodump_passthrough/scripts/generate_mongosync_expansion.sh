#!/bin/bash

set -o errexit
set -o xtrace

# shellcheck disable=SC2154
cat <<EOT >mongotools_expansion.yml
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
EOT
# See what we've done
cat mongotools_expansion.yml
