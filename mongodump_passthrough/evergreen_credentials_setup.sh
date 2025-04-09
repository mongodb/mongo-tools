#!/bin/bash

set -o errexit
set -o verbose

# TODO: Remove this once we have the right private variable in evergreen project settings.
evergreen_api_key=823845634ee1a616b7702cc1e24df96e
evergreen_api_user=richard.cownie

# Create the Evergreen API credentials
# shellcheck disable=SC2154
cat >.evergreen.yml <<END_OF_CREDS
api_server_host: https://evergreen.mongodb.com/api
api_key: "$evergreen_api_key"
user: "$evergreen_api_user"
END_OF_CREDS
