#!/bin/bash
# Copyright (c) 2018-Present MongoDB Inc.
echo $0

if [ "$PLATFORM" = "" ]; then
    PLATFORM=macos
    echo "WARNING: no value provided for \$PLATFORM: using default of '$PLATFORM'"
fi

case "$PLATFORM" in
ubuntu1404-64)
    PLATFORM_ARCH='64'
    PLATFORM_NAME='linux'
    MONGODB_URI='http://downloads.10gen.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1404-latest.tgz'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-linux-x86_64-ubuntu-1404-latest.tgz'
    EXTRACT='tar xf'
    CMAKE_GENERATOR="Unix Makefiles"
    CMAKE_PATH='/opt/cmake/bin'
    ;;
ubuntu1604-64)
    PLATFORM_ARCH='64'
    PLATFORM_NAME='linux'
    MONGODB_URI='http://downloads.10gen.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1604-latest.tgz'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-linux-x86_64-ubuntu-1604-latest.tgz'
    EXTRACT='tar xf'
    CMAKE_GENERATOR="Unix Makefiles"
    CMAKE_PATH='/opt/cmake/bin'
    ;;
rhel70)
    PLATFORM_ARCH='64'
    PLATFORM_NAME='linux'
    MONGODB_URI='http://downloads.10gen.com/linux/mongodb-linux-x86_64-enterprise-rhel70-latest.tgz'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-linux-x86_64-rhel70-latest.tgz'
    EXTRACT='tar xf'
    CMAKE_GENERATOR="Unix Makefiles"
    CMAKE_PATH='/opt/cmake/bin'
    ;;
macos)
    PLATFORM_ARCH='64'
    PLATFORM_NAME='macos'
    MONGODB_URI='http://downloads.10gen.com/osx/mongodb-osx-x86_64-enterprise-latest.tgz'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-osx-x86_64-latest.tgz'
    EXTRACT='tar xf'
    CMAKE_GENERATOR="Unix Makefiles"
    CMAKE_PATH='/Applications/Cmake.app/Contents/bin'
    ;;
win64)
    # We must use 64-bit powershell to test 64-bit odbc
    POWERSHELL='C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe' # 64-bit powershell
    MONGODB_URI='http://downloads.10gen.com/win32/mongodb-win32-x86_64-enterprise-windows-64-latest.zip'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-win32-x86_64-latest.msi'
    EXTRACT='unzip'
    PLATFORM_ARCH='64'
    PLATFORM_NAME='windows'
    CMAKE_GENERATOR="Visual Studio 14 2015 Win64"
    # make sure binaries we use in our scripts are available in the PATH
    DEVENV_PATH='/cygdrive/c/Program Files (x86)/Microsoft Visual Studio 14.0/Common7/IDE'
    CMAKE_PATH='/cygdrive/c/cmake/bin'
    WIX_PATH='/cygdrive/c/wixtools/bin'
	OPENSSL_PATH='/cygdrive/c/openssl/bin'
    ;;
win32)
    # We must use 32-bit powershell to test 32-bit odbc
    POWERSHELL='C:/Windows/SysWOW64/WindowsPowerShell/v1.0/powershell.exe' # 32-bit powershell
    MONGODB_URI='http://downloads.10gen.com/win32/mongodb-win32-x86_64-enterprise-windows-64-latest.zip'
    SQLPROXY_URI='https://info-mongodb-com.s3.amazonaws.com/mongodb-bi/v2/latest/mongodb-bi-win32-x86_64-latest.msi'
    EXTRACT='unzip'
    PLATFORM_ARCH='32'
    PLATFORM_NAME='windows'
    CMAKE_GENERATOR="Visual Studio 14 2015"
    # make sure binaries we use in our scripts are available in the PATH
    DEVENV_PATH='/cygdrive/c/Program Files (x86)/Microsoft Visual Studio 14.0/Common7/IDE'
    CMAKE_PATH='/cygdrive/c/cmake/bin'
    WIX_PATH='/cygdrive/c/wixtools/bin'
	OPENSSL_PATH='/cygdrive/c/openssl32/openssl-1_0_2k/bin'
    ;;
*)
    echo "ERROR: invalid value for \$PLATFORM: '$PLATFORM'"
    echo "Allowed values: 'ubuntu1604-64', 'rhel70', 'win64', 'win32', 'macos'"
    exit 1
    ;;
esac
