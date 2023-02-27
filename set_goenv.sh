#!/bin/bash

set_goenv() {
    # Error out if not in the same directory as this script
    if [ ! -f ./set_goenv.sh ]; then
        echo "Must be run from mongo-tools top-level directory. Aborting."
        return 1
    fi

    # Set OS-level default Go configuration
    UNAME_S=$(PATH="/usr/bin:/bin" uname -s)
    case $UNAME_S in
        CYGWIN*)
            PREF_GOROOT="c:/golang/go1.19"
            PREF_PATH="/cygdrive/c/golang/go1.19/bin:/cygdrive/c/mingw-w64/x86_64-4.9.1-posix-seh-rt_v3-rev1/mingw64/bin:$PATH"
        ;;
        *)
            PREF_GOROOT="/opt/golang/go1.19"
            PREF_PATH="$PREF_GOROOT/bin:$PATH"
        ;;
    esac

    # Set OS-level compilation flags
    case $UNAME_S in
        CYGWIN*)
            export CGO_CFLAGS="-D_WIN32_WINNT=0x0601 -DNTDDI_VERSION=0x06010000"
            export GOCACHE="C:/windows/temp"
            ;;
        Darwin)
            export CGO_CFLAGS="-mmacosx-version-min=10.11"
            export CGO_LDFLAGS="-mmacosx-version-min=10.11"
            ;;
    esac

    # On s390x, the Go toolchain relies on s390x-linux-gnu-gcc which isn't set up.
    # Temporarily rely on the mongodbtoolchain for compilation on s390x.
    if [ -z "$CC" ]; then
        UNAME_M=$(PATH="/usr/bin:/bin" uname -m)
        case $UNAME_M in
            s390x)
                export CC=/usr/bin/gcc
            ;;
            *)
                # Not needed for other architectures
            ;;
        esac
    fi

    # If GOROOT is not set by the user, configure our preferred Go version and
    # associated path if available or error.
    if [ -z "$GOROOT" ]; then
        if [ -d "$PREF_GOROOT" ]; then
            export GOROOT="$PREF_GOROOT";
            export PATH="$PREF_PATH";
        else
            echo "GOROOT not set and preferred GOROOT '$PREF_GOROOT' doesn't exist. Aborting."
            return 1
        fi
    fi

    # Derive GOPATH from current directory, but error if the current directory
    # doesn't look like a GOPATH structure.
    if expr "$(pwd)" : '.*src/github.com/mongodb/mongo-tools$' > /dev/null; then
        export GOPATH=$(echo $(pwd) | perl -pe 's{src/github.com/mongodb/mongo-tools}{}')
        if expr "$UNAME_S" : 'CYGWIN' > /dev/null; then
            export GOPATH=$(cygpath -w "$GOPATH")
        fi
    else
        echo "Current path '$(pwd)' doesn't resemble a GOPATH-style path. Aborting.";
        return 1
    fi

    return
}
