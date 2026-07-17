#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

: "${IAM_AUTH_ASSUME_AWS_ACCOUNT?}" "${IAM_AUTH_ASSUME_AWS_SECRET_ACCESS_KEY?}" "${IAM_AUTH_ASSUME_ROLE_NAME?}"

cat <<EOF >common/testdata/lib/aws_e2e_setup.json
{
  "iam_auth_assume_aws_account": "$IAM_AUTH_ASSUME_AWS_ACCOUNT",
  "iam_auth_assume_aws_secret_access_key": "$IAM_AUTH_ASSUME_AWS_SECRET_ACCESS_KEY",
  "iam_auth_assume_role_name": "$IAM_AUTH_ASSUME_ROLE_NAME"
}
EOF
