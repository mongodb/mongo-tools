#!/bin/bash

set -e
set -x

killall -q mongod || true

rm -fr "$HOME/var/*"

mkdir -p "$HOME/var/log"

for i in 0 1 2; do
    DATA_DIR="$HOME/var/lib/mongodb-rs-$i"
    mkdir -p "$DATA_DIR"
    LOG_FILE="$HOME/var/log/mongodb-rs-$i.log"
    port=$(expr 33333 + $i )
    m use 5.3.1 \
      --replSet rs0 \
      --port $port \
      --bind_ip localhost \
      --dbpath "$DATA_DIR" \
      --logpath "$LOG_FILE" \
      --oplogSize 128 \
      --fork
done

mongosh --port 33333 --eval 'rs.initiate({
  _id: "rs0",
  members: [
    {
     _id: 0,
     host: "localhost:33333",
     priority: 1000
    },
    {
     _id: 1,
     host: "localhost:33334"
    },
    {
     _id: 2,
     host: "localhost:33335"
    }
   ]
})'
