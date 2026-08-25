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
#
# Installs from the npm backend are the other exception. mise points bin/<tool> at the package's
# real script via a symlink, and Evergreen's cache.save dereferences symlinks while archiving, so a
# restored bin/<tool> is a plain copy one directory above where it expects to live. Anything it
# resolves relative to itself then misses (mongodb-runner's shim does `require("../dist/cli.js")`).
# Throwing those installs away makes the `mise install` below rebuild them with working symlinks.
# cache.save is skipped on a hit, so the repaired copies never get archived back.
if [ "${MISE_ALL_TOOLS_CACHE_HIT:-}" = "true" ]; then
    rm -rf "${MISE_DATA_DIR:?}/installs/npm-"*
fi

# We only retry twice here because each attempt uses up some of the GitHub API's rate limit. On a
# cache hit this reinstalls just the npm tools purged above; everything else is already present.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install

retry mise exec node -- npm install
