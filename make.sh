#!/bin/bash

# exit on error
set -e

TOOLS_PKG='github.com/mongodb/mongo-tools'

# buildable targets
TARGETS=(
    bsondump
    mongodump 
    mongoexport
    mongofiles
    mongoimport
    mongooplog
    mongorestore
    mongostat
    mongotop
    all
)

# the build target specified by the user
TARGET=$1

# helper function to check if an array contains an element
contains() {
    local resultvar=$1
    local target="$2"  
    local array="${@:3}"
    for el in $array
    do
        if [[ "$el" == "$target" ]]
        then
            eval $resultvar=1
            return 
        fi
    done
    eval $resultvar=0
}

contains found "$TARGET" "${TARGETS[@]}"
if [ $found == 0 ]
then
    echo "Invalid build target $1"
    exit 1
fi

# make sure we're in the directory where the make script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR
echo "Running mongo-tools build script..."

# set up the $GOPATH to be able to build the tools using the
# vendored dependencies
rm -rf .gopath/
mkdir -p .gopath/src/"$(dirname $TOOLS_PKG)"
ln -sf `pwd` .gopath/src/$TOOLS_PKG
export GOPATH=`pwd`/.gopath:`pwd`/vendor

# set up the target directory for the binaries
rm -rf bin
mkdir bin

# determine the directory, or directories, for the binaries we will build
if [ "$TARGET" == "all" ]
then
    TARGET_DIRS=() 
    for ((i=0;i<"${#TARGETS[@]}";i++))
    do
        target="${TARGETS[i]}" 
        if [ "$target" != "all" ]
        then
            TARGET_DIRS[i]="$target"/main
        fi
    done
else 
    TARGET_DIRS=( "$TARGET"/main )
fi

# build all necessary binaries
for ((i=0;i<"${#TARGET_DIRS[@]}";i++))
do
    target="${TARGET_DIRS[i]}" 
    withoutmain=$(dirname "$target")
    echo "Building $withoutmain..."
    cd "$target"
    go build "$withoutmain".go
    cd "$SCRIPT_DIR"
    cp "$target/$withoutmain" bin/
    echo "$withoutmain successfully built"
done
