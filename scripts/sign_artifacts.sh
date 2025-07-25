#!/bin/bash

set -o errexit

pgp_sign() {
  file_name=$1
  signature_name=$2

  podman run \
    --env-file=signing-envfile \
    --rm \
    --volume "$PWD:$PWD" \
    --workdir "$PWD" \
    artifactory.corp.mongodb.com/release-tools-container-registry-local/garasign-gpg \
    /bin/bash -c "gpgloader && gpg --yes -v --armor -o ${signature_name} --detach-sign ${file_name}"
}

authenticode_sign() {
  file_name=$1

  podman run \
  --env-file=signing-envfile \
  --rm \
  --volume "$PWD:$PWD" \
  --workdir "$PWD" \
  artifactory.corp.mongodb.com/release-tools-container-registry-local/garasign-jsign \
  /bin/bash -c "jsign -a ${AUTHENTICODE_KEY_NAME} --replace --tsaurl http://timestamp.digicert.com -d SHA-256 ${file_name}"
}

setup_garasign_authentication() {
  set +x

  echo "${ARTIFACTORY_PASSWORD}" | podman login --password-stdin --username "${ARTIFACTORY_USERNAME}" artifactory.corp.mongodb.com

  echo "GRS_CONFIG_USER1_USERNAME=${GARASIGN_USERNAME}" >> "signing-envfile"
  echo "GRS_CONFIG_USER1_PASSWORD=${GARASIGN_PASSWORD}" >> "signing-envfile"

  set -x
}

macos_sign_maybe_notarize() {
  set -o verbose

  tarball=$(ls mongodb-database-tools-*.tgz)

  # untar the release package and get the package name
  tar xvzf "$tarball"
  rm "$tarball"

  pkgname=$(basename -s .tgz "$tarball")

  # turn the untarred package into a zip
  zip -r unsigned.zip "$pkgname"

  uname_arch=$(uname -m)

  case "$uname_arch" in
    arm64)
      myarch=arm64
      ;;
    x86_64)
      myarch=amd64
      ;;
    *)
      echo "Unknown architecture: $uname_arch"
      exit 1
  esac

  if [ -n "$EVG_TRIGGERED_BY_TAG" ]
  then
    echo "This build was triggered by a Git tag ($EVG_TRIGGERED_BY_TAG). Will sign & notarize."
    notary_mode="notarizeAndSign"
  else
    echo "This build was not triggered by a Git tag. Will sign but not notarize."
    notary_mode="sign"
  fi

  macnotary_dir=darwin_${myarch}
  zip_filename=${macnotary_dir}.zip

  curl -LO "https://macos-notary-1628249594.s3.amazonaws.com/releases/client/v3.3.3/$zip_filename"
  unzip "$zip_filename"
  chmod 0755 "./$macnotary_dir/macnotary"
  "./$macnotary_dir/macnotary" -v

  # The key id and secret were set as MACOS_NOTARY_KEY and MACOS_NOTARY_SECRET
  # env vars from the expansions. The macnotary client will look for these env
  # vars so we don't need to pass the credentials as CLI options.
  "./$macnotary_dir/macnotary" \
      --task-comment "signing the mongo-database-tools release" \
      --task-id "$EVG_TASK_ID" \
      --file "$PWD/unsigned.zip" \
      --mode "${notary_mode}" \
      --url https://dev.macos-notary.build.10gen.cc/api \
      --bundleId com.mongodb.mongotools \
      --out-path "$PWD/$pkgname.zip"
}

case $MONGO_OS in
  "osx")
    macos_sign_maybe_notarize
    ;;

  "windows-64")
    setup_garasign_authentication
    msifile=$(ls mongodb-database-tools*.msi)
    authenticode_sign "$msifile"
    zipfile=$(ls mongodb-database-tools*.zip)
    pgp_sign "$zipfile" "$zipfile.sig"
    ;;

  *)
    setup_garasign_authentication
    for file in mongodb-database-tools*.{tgz,deb,rpm}; do
        [ -e "$file" ] || continue
        
        pgp_sign "$file" "$file.sig"
    done
    ;;
esac

ls -la
