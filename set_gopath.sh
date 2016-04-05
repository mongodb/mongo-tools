#!/bin/bash
MTAPE_PACKAGE='github.com/10gen/mongotape'

setgopath() {
    if [ "Windows_NT" != "$OS" ]; then
        SOURCE_GOPATH=`pwd`.gopath
        VENDOR_GOPATH=`pwd`/vendor

        # set up the $GOPATH to use the vendored dependencies as
        # well as the source 
        rm -rf .gopath/
        mkdir -p .gopath/src/"$(dirname "${MTAPE_PACKAGE}")"
        ln -sf `pwd` .gopath/src/$MTAPE_PACKAGE
        export GOPATH=`pwd`/vendor:`pwd`/.gopath

    else
        echo "using windows + cygwin gopath [COPY ONLY]"
        local SOURCE_GOPATH=`pwd`/.gopath
        local VENDOR_GOPATH=`pwd`/vendor
        SOURCE_GOPATH=$(cygpath -w $SOURCE_GOPATH);
        VENDOR_GOPATH=$(cygpath -w $VENDOR_GOPATH);

        # set up the $GOPATH to use the vendored dependencies as
        # well as the source for the mongo tools
        rm -rf .gopath/
        mkdir -p .gopath/src/"$MTAPE_PACKAGE"
        cp -r `pwd`/* .gopath/src/$MTAPE_PACKAGE
        # now handle vendoring
        rm -rf .gopath/src/$MTAPE_PACKAGE/vendor 
        cp -r `pwd`/vendor/src/* .gopath/src/.
        export GOPATH="$SOURCE_GOPATH;$VENDOR_GOPATH"
    fi;
}

setgopath
