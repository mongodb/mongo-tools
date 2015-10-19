#!/usr/bin/env bash

set -e

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR
echo "Installing dependencies..."

# set the $GOPATH appropriately. as the first entry in the $GOPATH,
# the dependencies will be installed in vendor/
. ./set_gopath.sh

# make sure the Godeps file is there
deps_file="Godeps"
[[ -f "$deps_file" ]] || (echo ">> $deps_file file does not exist." && exit 1)

# make sure go is installed
(go version > /dev/null) ||
  ( echo ">> Go is currently not installed or in your PATH" && exit 1)

# clean out the vendor directory
rm -rf vendor/*

# iterate over Godep file dependencies and set
# the specified version on each of them.
while read line; do
  linesplit=`echo $line | sed 's/#.*//;/^\s*$/d' || echo ""`
  [ ! "$linesplit" ] && continue
  (
    cd $SCRIPT_DIR

    linearr=($linesplit)
    package=${linearr[0]}
    version=${linearr[1]}
    importpath=${linearr[2]}

    install_path="vendor/src/${package%%/...}"

    [[ -e "$install_path/.git/index.lock" ||
       -e "$install_path/.hg/store/lock"  ||
       -e "$install_path/.bzr/checkout/lock" ]] && wait

    echo ">> Getting package "$package""
    go get -d "$package"

    cd $install_path
    hg update     "$version" > /dev/null 2>&1 || \
    git checkout  "$version" > /dev/null 2>&1 || \
    bzr revert -r "$version" > /dev/null 2>&1 || \
    #svn has exit status of 0 when there is no .svn
    { [ -d .svn ] && svn update -r "$version" > /dev/null 2>&1; } || \
    { echo ">> Failed to set $package to version $version"; exit 1; }

    echo ">> Set $package to version $version"

    if [[ "${importpath}" != "" ]]
    then
        cd $SCRIPT_DIR
        echo ">> Moving package to import path ${importpath}"
        importdirup="$(dirname "${importpath}")" 
        mkdir -p vendor/src/${importdirup}
        rm -rf vendor/src/${importpath}
        mv ${install_path} vendor/src/${importdirup}
        echo ">> Package moved"
    fi

  ) 
done < $deps_file

# remove all revision control info
find vendor -name ".git" | xargs rm -rf
find vendor -name ".svn" | xargs rm -rf
find vendor -name ".bzr" | xargs rm -rf

echo ">> All Done"
