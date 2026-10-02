#!/usr/bin/env bash

# Installs the node runtime and mongodb-runner, the two mise tools the cluster tasks need, so they
# can be cached separately from "install mise-managed tools" (which installs every tool).

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

recreate_npm_bin_symlinks

# Cache hit: node and mongodb-runner were already restored from S3, nothing to install.
if [ "${MISE_MONGODB_RUNNER_CACHE_HIT:-}" = "true" ]; then
    exit 0
fi

# We only retry twice here because each attempt uses up some of the GitHub API's rate limit.
RETRY_FAILURES_BEFORE_BACKOFF=0 RETRY_FAILURES_BEFORE_HARD_FAIL=1 \
    retry mise install node npm:@mongodb-js/mongodb-runner

# mongodb-runner declares a `mongodb` dependency range that a fresh install resolves to a newer 7.x,
# which dropped MongoDB 4.2 support (its Node driver requires wire version 9 / MongoDB 4.4). Pin
# 7.5.0, the last 7.x that supports 4.2, so the runner can manage our 4.2 tasks. This pairs with the
# legacy-log shim in start-test-cluster.sh. Remove both once mongodb-runner supports 4.2 itself.
for runner_pkg in "${MISE_DATA_DIR:?}"/installs/npm-mongodb-js-mongodb-runner/*/lib/node_modules/@mongodb-js/mongodb-runner; do
    [ -d "$runner_pkg" ] || continue
    (
        cd "$runner_pkg"
        mise exec node -- npm install --no-save --no-audit --no-fund --ignore-scripts mongodb@7.5.0
    )
done
