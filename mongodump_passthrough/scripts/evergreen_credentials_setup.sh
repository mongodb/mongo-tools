#!/bin/bash

set -o errexit
set -o verbose

# Create the Evergreen API credentials
# shellcheck disable=SC2154
cat >.evergreen.yml <<END_OF_CREDS
api_server_host: https://evergreen.mongodb.com/api
api_key: "$evergreen_api_key"
user: "$evergreen_api_user"
END_OF_CREDS
