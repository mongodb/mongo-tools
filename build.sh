#!/bin/bash
set -o errexit
tags=""
if [ ! -z "$1" ]
  then
  	tags="$@"
fi

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

. ./set_goenv.sh
set_goenv || exit

BINARY_EXT=""
UNAME_S=$(PATH="/usr/bin:/bin" uname -s)
    case ${UNAME_S} in
        CYGWIN*)
            BINARY_EXT=".exe"
        ;;
    esac

# remove stale packages
rm -rf vendor/pkg

mkdir -p bin

ec=0
for i in bsondump mongostat mongofiles mongoexport mongoimport mongorestore mongodump mongotop mongoreplay; do
        echo "Building ${i}..."
        go build -o "bin/$i$BINARY_EXT" $(buildflags) -ldflags "$(print_ldflags)" -tags "$(print_tags $tags)" "$i/main/$i.go" || { echo "Error building $i"; ec=1; break; }
        ./bin/${i}${BINARY_EXT} --version | head -1
done

if [ -t /dev/stdin ]; then
    stty sane
fi

exit $ec
