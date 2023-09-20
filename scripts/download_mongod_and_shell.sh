#!/bin/bash

# This is a small wrapper around the python script to translate env vars into CLI args for the
# python script.

if [ -n "$mongo_target" ]; then
    target="$mongo_target"
else
    target="$mongo_os"
fi

if [ -n "$mongo_arch" ]; then
    arch="$mongo_arch"
else
    arch="x86_64"
fi

if [ "$mongo_edition" = "enterprise" ]; then
    edition="enterprise"
fi

./scripts/download_mongod_and_shell.py \
    --arch "$arch" \
    --edition "$edition" \
    --target "$target" \
    --version "$mongo_version"
