#!/bin/bash
set -e

progpath=$(dirname $0)

toolbin=$1
if [ -z "$toolbin" ]; then
    toolbin="$progpath/../../bin"
fi

mongobin=$2
if [ -z "$mongobin" ]; then
    prog=$(which mongod)
    if [ -z "$prog" ]; then
        echo "Couldn't find $prog"
        exit 1
    fi
    mongobin=$(dirname $prog)
fi

echo "Copying tools from $toolbin"

for i in bsondump mongostat mongofiles mongoexport mongoimport mongorestore mongodump mongotop mongoreplay; do
    f="$toolbin/$i"
    echo "  - $(basename $f)"
    cp $f $progpath
done

echo "Copying mongod, mongos and mongo from $mongobin"
for p in mongo mongos mongod; do
    prog="$mongobin/$p"
    echo "  - $(basename $prog)"
    cp $prog $progpath
done
