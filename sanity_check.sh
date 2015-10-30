#!/bin/bash

PORT=27017
PCAPFILE="mongoplay_test.out"
STARTMONGO=false

set -e
set -v
while test $# -gt 0; do
	case "$1" in
		-f|--file)
			shift
			PCAPFILE="$1"
			shift
			;;
		-p|--port)
			shift
			PORT="$1"
			shift
			;;
		-m|--start-mongo)
			shift
			STARTMONGO=true
			;;
		*)
			echo "Unknown arg: $1"
			exit 1
	esac
done

OUTFILE="$(echo $PCAPFILE | cut -f 1 -d '.').playback"
./mongoplay record -f $PCAPFILE $OUTFILE

if [ $STARTMONGO ]; then
	rm -rf /data/mongoplay/
	mkdir /data/mongoplay/
	echo "starting mongod"
	mongod --port=$PORT --dbpath=/data/mongoplay > /dev/null 2>&1 &
	MONGOPID=$!
fi

mongo --port=$PORT mongoplay_test --eval "db.setProfilingLevel(2);"
mongo --port=$PORT mongoplay_test --eval "db.createCollection('sanity_check', {});"

./mongoplay play $OUTFILE
mongo --port=$PORT mongoplay_test --eval "var profile_results = db.system.profile.find({'ns':'mongoplay_test.sanity_check'});
assert.gt(profile_results.size(), 0);"

mongo --port=$PORT mongoplay_test --eval "var query_results = db.sanity_check.find({"test_success":1});
assert.gt(query_results.size(), 0);"
echo "Success!"

if [ $STARTMONGO ]; then
	kill $MONGOPID
fi


