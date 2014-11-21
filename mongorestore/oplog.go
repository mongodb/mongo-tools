package mongorestore

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"os"
	"strconv"
	"strings"
	"time"
)

const OplogMaxCommandSize = 1024 * 1024 * 16.5

type Oplog struct {
	Timestamp bson.MongoTimestamp `bson:"ts,omitempty"`
	HistoryID int64               `bson:"h,omitempty"`
	Version   int                 `bson:"v,omitempty"`
	Operation string              `bson:"op,omitempty"`
	Namespace string              `bson:"ns,omitempty"`
	Object    bson.M              `bson:"o,omitempty"`
	Query     bson.M              `bson:"o2,omitempty"`
}

func (restore *MongoRestore) RestoreOplog() error {
	log.Log(log.Always, "replaying oplog")
	intent := restore.manager.Oplog()
	if intent == nil {
		// this should not be reached
		log.Log(log.Always, "no oplog.bson file in root of the dump directory, skipping oplog application")
		return nil
	}

	fileInfo, err := os.Lstat(intent.BSONPath)
	if err != nil {
		return fmt.Errorf("error reading bson file: %v", err)
	}
	size := fileInfo.Size()
	log.Logf(log.Info, "\toplog %v is %v bytes", intent.BSONPath, size)

	oplogFile, err := os.Open(intent.BSONPath)
	if err != nil {
		return fmt.Errorf("error reading oplog file: %v", err)
	}

	bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(oplogFile))
	defer bsonSource.Close()

	entryArray := make([]interface{}, 0, 1024)
	rawOplogEntry := &bson.Raw{}

	var totalBytes, totalOps int64
	var entrySize, bufferedBytes int

	bar := progress.ProgressBar{
		Name:       "oplog",
		Max:        size,
		CounterPtr: &totalBytes,
		WaitTime:   3 * time.Second,
		Writer:     log.Writer(0),
		BarLength:  ProgressBarLength,
	}
	bar.Start()
	defer bar.Stop()

	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	session.SetSocketTimeout(0)
	defer session.Close()

	// To restore the oplog, we iterate over the oplog entries,
	// filling up a buffer. Once the buffer reaches max document size,
	// apply the current buffered ops and reset the buffer.
	for bsonSource.Next(rawOplogEntry) {
		entrySize = len(rawOplogEntry.Data)
		if bufferedBytes+entrySize > OplogMaxCommandSize {
			err = restore.ApplyOps(session, entryArray)
			if err != nil {
				return fmt.Errorf("error applying oplog: %v", err)
			}
			entryArray = make([]interface{}, 0, 1024)
			bufferedBytes = 0
		}

		entryAsOplog := Oplog{}
		err = bson.Unmarshal(rawOplogEntry.Data, &entryAsOplog)
		if err != nil {
			return fmt.Errorf("error reading oplog: %v", err)
		}
		if entryAsOplog.Operation == "n" {
			//skip no-ops
			continue
		}
		if !restore.TimestampBeforeLimit(entryAsOplog.Timestamp) {
			log.Logf(
				log.DebugLow,
				"timestamp %v is not below limit of %v; ending oplog restoration",
				entryAsOplog.Timestamp,
				restore.oplogLimit,
			)
			break
		}

		totalOps++
		bufferedBytes += entrySize
		totalBytes += int64(entrySize)
		entryArray = append(entryArray, entryAsOplog)
	}
	// finally, flush the remaining entries
	if len(entryArray) > 0 {
		err = restore.ApplyOps(session, entryArray)
		if err != nil {
			return fmt.Errorf("error applying oplog: %v", err)
		}
	}

	log.Logf(log.Info, "applied %v ops", totalOps)
	return nil

}

// ApplyOps is a wrapper for the applyOps database command, we pass in
// a session to avoid opening a new connection for a few inserts at a time
func (restore *MongoRestore) ApplyOps(session *mgo.Session, entries []interface{}) error {
	res := bson.M{}
	err := session.Run(bson.D{{"applyOps", entries}}, &res)
	if err != nil {
		return fmt.Errorf("applyOps: %v", err)
	}
	if util.IsFalsy(res["ok"]) {
		return fmt.Errorf("applyOps command: %v", res["errmsg"])
	}

	return nil
}

// OplogTimestampIsValid returns whether the given timestamp is allowed to be
// applied to mongorestore's target database.
func (restore *MongoRestore) TimestampBeforeLimit(ts bson.MongoTimestamp) bool {
	if restore.oplogLimit == 0 {
		// always valid if there is no --oplogLimit set
		return true
	}
	return ts < restore.oplogLimit
}

// ParseTimestampFlag takes in a string the form of <time_t>:<ordinal>,
// where <time_t> is the seconds since the UNIX epoch, and <ordinal> represents
// a counter of operations in the oplog that occurred in the specified second.
// It parses this timestamp string and returns a bson.MongoTimestamp type.
func ParseTimestampFlag(ts string) (bson.MongoTimestamp, error) {
	var seconds, increment int
	timestampFields := strings.Split(ts, ":")
	if len(timestampFields) > 2 {
		return 0, fmt.Errorf("too many : characters")
	}

	seconds, err := strconv.Atoi(timestampFields[0])
	if err != nil {
		return 0, fmt.Errorf("error parsing timestamp seconds: %v", err)
	}

	// parse the increment field if it exists
	if len(timestampFields) == 2 {
		if len(timestampFields[1]) > 0 {
			increment, err = strconv.Atoi(timestampFields[1])
			if err != nil {
				return 0, fmt.Errorf("error parsing timestamp increment: %v", err)
			}
		} else {
			// handle the case where the user writes "<time_t>:" with no ordinal
			increment = 0
		}
	}

	timestamp := (int64(seconds) << 32) | int64(increment)
	return bson.MongoTimestamp(timestamp), nil
}
