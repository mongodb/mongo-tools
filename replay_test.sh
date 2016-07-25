#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PORT=28080
INTERFACE=lo
KEEP=false
DBPATH=/data/mongotape
SILENT="--silent"
VERBOSE=
DEBUG=
WORKLOAD="$SCRIPT_DIR/testPcap/crud.js"
MONGOASSERT='db.bench.count() === 15000'
CUSTOM_WORKLOAD=0

log() {
  >&2 echo $@
}

while test $# -gt 0; do
  case "$1" in
    -h|--help)
      >&2 cat <<< "Usage: `basename $0` [OPTIONS]

-a, --assert JS-BOOL     condition for assertion after workload (used with -w);
                         (defaults to $MONGOASSERT)
    --dbpath             path for mongod
-i, --interface NI       network interface (defaults to $INTERFACE)
-k, --keep               keep temp files
-p, --port PORT          use port PORT (defaults to $PORT)
-v, --verbose            unsilence mongotape and make it slightly loud
-w, --workload JS-FILE   mongo shell workload script (used with -a);
                         must only interact with 'bench' collection
                         (defaults to $WORKLOAD)
"
      exit 1
      ;;
    -a|--assert)
      shift
      MONGOASSERT="$1"
      CUSTOM_WORKLOAD=$((CUSTOM_WORKLOAD+1))
      shift
      ;;
    -i|--interface)
      shift
      INTERFACE="$1"
      shift
      ;;
    -k|--keep)
      shift
      KEEP=true
      ;;
    -p|--port)
      shift
      PORT="$1"
      shift
      ;;
    -v|--verbose)
      shift
      SILENT=
      VERBOSE="-vv"
      DEBUG="-dd"
      ;;
    -w|--workload)
      shift
      WORKLOAD="$1"
      CUSTOM_WORKLOAD=$((CUSTOM_WORKLOAD+1))
      shift
      ;;
    --dbpath)
      shift
      DBPATH="$1"
      shift
      ;;
    *)
      log "Unknown arg: $1"
      exit 1
  esac
done

if [ $CUSTOM_WORKLOAD == 1 ]; then
  log "must specify BOTH -a/--assert AND -w/--workload"
  exit 1
fi

command -v mongotape >/dev/null
if [ $? != 0 ]; then
  log "mongotape must be in PATH"
  exit 1
fi
command -v ftdc >/dev/null
if [ $? != 0 ]; then
  log "ftdc command (github.com/10gen/ftdc-utils) must be in PATH"
  exit 1
fi

set -e

rm -rf $DBPATH
mkdir $DBPATH
log "starting MONGOD"
mongod --port=$PORT --dbpath=$DBPATH >/dev/null &
MONGOPID=$!

check_integrity() {
  set +e
  mongo --port=$PORT --quiet mongotape_test --eval "assert($MONGOASSERT)" >/dev/null
  STATUS=$?
  set -e
  if [ $STATUS != 0 ]; then
    log "integrity check FAILED: $MONGOASSERT"
    log "for further analysis, check db at localhost:$PORT, pid=$MONGOPID"
    exit 1
  fi
}

sleep 1
mongo --port=$PORT mongotape_test --eval "db.bench.drop()" >/dev/null 2>&1

log "starting mongotape RECORD"
mongotape record $SILENT $VERBOSE $DEBUG -i=$INTERFACE -p=tmp.playback >/dev/null &
TAPEPID=$!
sleep 1 # make sure it actually starts recording

log "starting CRUD"
START=`date`
sleep 1
mongo --port=$PORT mongotape_test "$WORKLOAD" >/dev/null
sleep 1
END=`date`
log "finished CRUD"

log "stopping mongotape RECORD"
( sleep 1 ; kill $TAPEPID) &
wait $TAPEPID
TAPECODE=$?
if [ "$TAPECODE" != 0 ]; then
  log "mongotape failed with code $TAPECODE"
  exit 1
fi

check_integrity

# clean up database
mongo --port=$PORT mongotape_test --eval "db.bench.drop()" >/dev/null 2>&1
sleep 1 # mongotape play should certainly happen after the drop

log # newline to separate replay

log "starting mongotape PLAY"
REPLAY_START=`date`
sleep 1
mongotape play $SILENT $VERBOSE $DEBUG --host "localhost:$PORT" -p=tmp.playback >/dev/null
sleep 1
REPLAY_END=`date`
log "finished mongotape PLAY"

check_integrity

log "flushing FTDC diagnostic files (15 sec)"
sleep 15

log "killing MONGOD"
mongo --port=$PORT mongotape_test --eval "db.bench.drop()" >/dev/null 2>&1
sleep 1
kill $MONGOPID
sleep 2 # give it a chance to dump FTDC

log "gathering FTDC statistics"
log -n "base:   "
ftdc stats $DBPATH/diagnostic.data/* --start="$START" --end="$END" --out="tmp.base.stat.json"
log -n "replay: "
ftdc stats $DBPATH/diagnostic.data/* --start="$REPLAY_START" --end="$REPLAY_END" --out="tmp.play.stat.json"

set +e
ftdc compare tmp.base.stat.json tmp.play.stat.json
CODE=$?

if [ "$KEEP" = false ]; then
  rm tmp.playback
  rm tmp.base.stat.json
  rm tmp.play.stat.json
fi
exit $CODE
