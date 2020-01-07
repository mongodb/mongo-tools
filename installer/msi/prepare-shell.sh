#!/bin/bash
# Copyright (c) 2018-Present MongoDB Inc.
# This file should be sourced by all scripts in mongodb-odbc-driver/bin

# we start by sourcing platforms.sh. this will set environment variables that
# differ depending on which platform we are building on
# shellcheck source=platforms.sh
. "$(dirname "$0")/platforms.sh"

set -o verbose

# create variables for a number of useful directories
SCRIPT_DIR=$(cd $(dirname $0) && pwd -P)
if [ "$OS" = "Windows_NT" ]; then
    SCRIPT_DIR="$(cygpath -m "$SCRIPT_DIR")"
fi

PROJECT_ROOT="$SCRIPT_DIR/../.."

basename=${0##*/}
ARTIFACTS_DIR="$PROJECT_ROOT/mongodb-odbc-driver/artifacts"
BUILD_DIR="$PROJECT_ROOT/mongodb-odbc-driver/build"
BUILD_SRC_DIR="$PROJECT_ROOT/mongodb-odbc-driver/src"
DMG_BUILD_DIR="$PROJECT_ROOT/mongodb-odbc-driver/dmg-build"
LOG_FILE="$ARTIFACTS_DIR/log/${basename%.sh}.log"
MDBODBC_VER="$(cat "$SCRIPT_DIR"/VERSION.txt)"
MSI_BUILD_DIR="$PROJECT_ROOT/mongodb-odbc-driver/msi-build"
PROJECT_DIR="$(dirname "$(dirname $SCRIPT_DIR)")"

# if on cygwin, convert paths as needed
if [ "Windows_NT" = "$OS" ]; then
    ARTIFACTS_DIR="$(cygpath -m $ARTIFACTS_DIR)"
    LOG_FILE="$(cygpath -m $LOG_FILE)"
    PROJECT_DIR="$(cygpath -m $PROJECT_DIR)"
    SCRIPT_DIR="$(cygpath -m $SCRIPT_DIR)"
fi

echo "setting up repo for testing..."
mkdir -p $ARTIFACTS_DIR/{bin,build,log}
echo "done setting up repo for testing"

CMAKE_MODULE_PATH="$BUILD_SRC_DIR/cmake"
DRIVERS_DIR="$ARTIFACTS_DIR/drivers"
IODBC_VERSION=iODBC-3.52.12
IODBC_BUILD_DIR="$BUILD_DIR"/"$IODBC_VERSION"/mac
IODBCTEST_PATH="$IODBC_BUILD_DIR"/iODBCtest/build/Deployment
IODBCTESTW_PATH="$IODBC_BUILD_DIR"/iODBCtestw/build/Deployment
MONGODB_DIR="$ARTIFACTS_DIR/mongodb"
MYSQL_PROJECT_DIR="$PROJECT_ROOT/mongodb-odbc-driver/libmongosql"
MYSQL_SCRIPT_DIR="$MYSQL_PROJECT_DIR/bld/bin"
MYSQL_DIR="$MYSQL_PROJECT_DIR/bld/artifacts/mysql-home"
MONGOSQL_AUTH_PROJECT_DIR="$MYSQL_PROJECT_DIR/bld/mongosql-auth-c"
PKG_DIR="$ARTIFACTS_DIR/pkg"
SQLPROXY_DIR="$ARTIFACTS_DIR/mongosqld"

PATH="$PATH:$DEVENV_PATH:$CMAKE_PATH:$WIX_PATH:$IODBCTEST_PATH:$IODBCTESTW_PATH"

# export any environment variables that will be needed by subprocesses
export CMAKE_MODULE_PATH
export IODBC_VERSION
export IODBC_BUILD_DIR
export MDBODBC_VER
export MYSQL_DIR

# Each script should run with errexit set and should start in the project root.
# In general, scripts should reference directories via the provided environment
# variables instead of making assumptions about the working directory.
cd "$PROJECT_ROOT"

# define the function that prints the exit message at the end of each script
print_exit_msg() {
    exit_code=$?
    if [ "$exit_code" != "0" ]; then
        status=FAILURE
    else
        status=SUCCESS
    fi

    echo "$status: $basename" 1>&2
    if [ "$status" = "FAILURE" ]; then
        echo "printing log from failed script:" 1>&2
        cat $LOG_FILE 1>&2
    fi

    return $exit_code
}
