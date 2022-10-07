#!/bin/bash

ls -la

# turn on verbose debugging for remainder of script
set -o xtrace
set -o errexit
set -o verbose

# this function should do nothing on non-mac platforms
if [ "${mongo_os}" != "osx" ]; then
    exit 0
fi

# untar the release package and get the package name
tar xvzf release.tgz
ls -la
pkgname=$(ls | grep mongodb-database-tools)
rm release.tgz

# turn the untarred package into a zip
zip -r unsigned.zip "$pkgname"

curl -LO https://macos-notary-1628249594.s3.amazonaws.com/releases/client/v3.3.3/darwin_amd64.zip
unzip darwin_amd64.zip
chmod 0755 ./darwin_amd64/macnotary
./darwin_amd64/macnotary -v

# The key id and secret were set as MACOS_NOTARY_KEY and MACOS_NOTARY_SECRET
# env vars from the expansions. The macnotary client will look for these env
# vars so we don't need to pass the credentials as CLI options.
./darwin_amd64/macnotary \
    --task-comment "signing the mongo-database-tools release" \
    --task-id "$TASK_ID" \
    --file "$PWD/unsigned.zip" \
    --mode notarizeAndSign \
    --url https://dev.macos-notary.build.10gen.cc/api \
    --bundleId com.mongodb.mongotools \
    --out-path "$PWD/release.zip"
