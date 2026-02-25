#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck disable=SC1091
source "$SCRIPT_DIR/functions.sh"

# Evergreen's ${workdir} expansion on Windows sometimes lacks a drive letter (e.g. "\data\mci\9f92"),
# and Go's os/exec refuses to run an executable found via a PATH entry it can't prove is absolute.
# cygpath -am always produces an absolute path with a drive letter.
case "$(uname -s)" in
CYGWIN* | MINGW* | MSYS*)
    EVG_WORKDIR=$(cygpath -am "${EVG_WORKDIR:?}")
    ;;
esac

export PATH="${EVG_WORKDIR:?}/.local/bin:$PATH"

export MISE_DATA_DIR="${EVG_WORKDIR:?}/.local/share/mise"

# Cache hit: .local/bin and .local/share/mise were already restored from S3, so there's no need to
# reinstall those. We don't cache node_modules though (see the cache.save comment in common.yml), so
# `npm install` always has to run, cache hit or not.
if [ "${MISE_ALL_TOOLS_CACHE_HIT:-}" != "true" ]; then
    # We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
    RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
        retry mise install
fi

retry mise exec node -- npm install
