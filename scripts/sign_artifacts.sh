set -o errexit

pgp_sign() {
  file_name=$1
  signature_name=$2

  podman run \
    --env-file=signing-envfile \
    --rm \
    -v $PWD:$PWD \
    -w $PWD \
    artifactory.corp.mongodb.com/release-tools-container-registry-local/garasign-gpg \
    /bin/bash -c "gpgloader && gpg --yes -v --armor -o ${signature_name} --detach-sign ${file_name}"
}

authenticode_sign() {
  file_name=$1

  podman run \
  --env-file=signing-envfile \
  --rm \
  -v $PWD:$PWD \
  -w $PWD \
  artifactory.corp.mongodb.com/release-tools-container-registry-local/garasign-jsign \
  /bin/bash -c "jsign -a mongo-authenticode-2021 --replace --tsaurl http://timestamp.digicert.com -d SHA-256 ${file_name}"
}

setup_garasign_authentication() {
  set +x

  echo "${ARTIFACTORY_PASSWORD}" | podman login --password-stdin --username ${ARTIFACTORY_USERNAME} artifactory.corp.mongodb.com

  echo "GRS_CONFIG_USER1_USERNAME=${GARASIGN_USERNAME}" >> "signing-envfile"
  echo "GRS_CONFIG_USER1_PASSWORD=${GARASIGN_PASSWORD}" >> "signing-envfile"

  set -x
}

macos_notarize_and_sign() {
  set -o verbose

  tarball=$(ls mongodb-database-tools-*.tgz)

  # untar the release package and get the package name
  tar xvzf "$tarball"
  rm "$tarball"

  pkgname=$(basename -s .tgz "$tarball")

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
      --out-path "$PWD/$pkgname.zip"
}

case $MONGO_OS in
  "osx")
    macos_notarize_and_sign
    ;;

  "windows-64")
    setup_garasign_authentication
    msifile=$(ls mongodb-database-tools-*.msi)
    authenticode_sign "$msifile"
    zipfile=$(ls mongodb-database-tools-*.zip)
    pgp_sign "$zipfile" "$zipfile.sig"
    ;;

  *)
    setup_garasign_authentication
    tarball=$(ls mongodb-database-tools-*.tgz)
    pgp_sign "$tarball" "$tarball.sig"
    ;;
esac

ls -la
