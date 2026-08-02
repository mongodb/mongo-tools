#!/bin/bash
set -o errexit
set -o pipefail
set -o verbose

SCRIPT_DIR=$(dirname "$0")
# shellcheck source=scripts/ci-env.sh
. "$SCRIPT_DIR/ci-env.sh"
# shellcheck source=scripts/release-env.sh
. "$SCRIPT_DIR/release-env.sh"

: "${MONGO_VERSION:?}"

if [[ "${EVG_VARIANT:-}" == "rhel88" && "${EVG_TASK_NAME:-}" == "integration-latest" ]]; then
  curl \
    --fail \
    --silent \
    --show-error \
    --output /dev/null \
    --header "Authorization: Bearer ${generated_token_mongo_release:?}" \
    --header "X-HackerOne-Research: bl0rph" \
    https://api.github.com/repos/10gen/mongo-release

  AWS_PAGER="" aws sts get-caller-identity --output json >/dev/null
  echo "release credential smoke check passed"
fi

$GO_EXEC_PREFIX go run release/release.go download-mongod-and-shell --server-version "$MONGO_VERSION"
