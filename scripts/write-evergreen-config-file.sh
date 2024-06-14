#!/bin/bash

# We don't want to print out the Evergreen key (though it should be redacted
# by Evergreen anyway).
set +x

cat <<EOF > "$HOME/.evergreen.yml"
user: "$EVG_USER"
api_key: "$EVG_KEY"
api_server_host: "https://evergreen.mongodb.com/api"
ui_server_host: "https://evergreen.mongodb.com"
EOF
