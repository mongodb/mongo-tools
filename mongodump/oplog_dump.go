// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"fmt"

	"github.com/mongodb/mongo-tools-common/bsonutil"
	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/util"
	gbson "go.mongodb.org/mongo-driver/bson"
	"gopkg.in/mgo.v2/bson"
)

// determineOplogCollectionName uses a command to infer
// the name of the oplog collection in the connected db
func (dump *MongoDump) determineOplogCollectionName() error {
	masterDoc := bson.M{}
	err := dump.SessionProvider.RunString("isMaster", &masterDoc, "admin")
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}
	if _, ok := masterDoc["hosts"]; ok {
		log.Logvf(log.DebugLow, "determined cluster to be a replica set")
		log.Logvf(log.DebugHigh, "oplog located in local.oplog.rs")
		dump.oplogCollection = "oplog.rs"
		return nil
	}
	if isMaster := masterDoc["ismaster"]; util.IsFalsy(isMaster) {
		log.Logvf(log.Info, "mongodump is not connected to a master")
		return fmt.Errorf("not connected to master")
	}

	log.Logvf(log.DebugLow, "not connected to a replica set, assuming master/slave")
	log.Logvf(log.DebugHigh, "oplog located in local.oplog.$main")
	dump.oplogCollection = "oplog.$main"
	return nil

}

// getOplogCurrentTime returns the most recent oplog entry
func (dump *MongoDump) getCurrentOplogTime() (bson.MongoTimestamp, error) {
	mostRecentOplogEntry := db.Oplog{}
	var tempBSON gbson.Raw

	err := dump.SessionProvider.FindOne("local", dump.oplogCollection, 0, nil, &bson.M{"$natural": -1}, &tempBSON, 0)
	if err != nil {
		return 0, err
	}
	err = gbson.Unmarshal(tempBSON, &mostRecentOplogEntry)
	if err != nil {
		return 0, err
	}
	return bsonutil.ConvertTimestampToMongoTimestamp(mostRecentOplogEntry.Timestamp), nil
}

// checkOplogTimestampExists checks to make sure the oplog hasn't rolled over
// since mongodump started. It does this by checking the oldest oplog entry
// still in the database and making sure it happened at or before the timestamp
// captured at the start of the dump.
func (dump *MongoDump) checkOplogTimestampExists(ts bson.MongoTimestamp) (bool, error) {
	oldestOplogEntry := db.Oplog{}
	var tempBSON gbson.Raw

	err := dump.SessionProvider.FindOne("local", dump.oplogCollection, 0, nil, &bson.M{"$natural": 1}, &tempBSON, 0)
	if err != nil {
		return false, fmt.Errorf("unable to read entry from oplog: %v", err)
	}
	err = gbson.Unmarshal(tempBSON, &oldestOplogEntry)
	if err != nil {
		return false, err
	}

	log.Logvf(log.DebugHigh, "oldest oplog entry has timestamp %v", oldestOplogEntry.Timestamp)
	if bsonutil.ConvertTimestampToMongoTimestamp(oldestOplogEntry.Timestamp) > ts {
		log.Logvf(log.Info, "oldest oplog entry of timestamp %v is older than %v",
			oldestOplogEntry.Timestamp, ts)
		return false, nil
	}
	return true, nil
}

func oplogDocumentFilter(in []byte) ([]byte, error) {
	var rawD bson.RawD
	err := bson.Unmarshal(in, &rawD)
	if err != nil {
		return nil, err
	}

	var nsD struct {
		NS string `bson:"ns"`
	}
	err = bson.Unmarshal(in, &nsD)
	if err != nil {
		return nil, err
	}

	if nsD.NS == "admin.system.version" {
		return nil, fmt.Errorf("cannot dump with oplog if admin.system.version is modified")
	}

	for i := range rawD {
		if rawD[i].Name == "o" {
			var rawO bson.RawD
			err = bson.Unmarshal(rawD[i].Value.Data, &rawO)
			for j := range rawO {
				if rawO[j].Name == "renameCollection" {
					return nil, fmt.Errorf("cannot dump with oplog while renames occur")
				}
			}
		}
	}
	return bson.Marshal(rawD)
}

// DumpOplogBetweenTimestamps takes two timestamps and writer and dumps all oplog
// entries between the given timestamp to the writer. Returns any errors that occur.
func (dump *MongoDump) DumpOplogBetweenTimestamps(start, end bson.MongoTimestamp) error {
	session, err := dump.SessionProvider.GetSession()
	if err != nil {
		return err
	}
	queryObj := bson.M{"$and": []bson.M{
		bson.M{"ts": bson.M{"$gte": start}},
		bson.M{"ts": bson.M{"$lte": end}},
	}}
	oplogQuery := &db.DeferredQuery{
		Coll:      session.Database("local").Collection(dump.oplogCollection),
		Filter:    queryObj,
		LogReplay: true,
	}
	oplogCount, err := dump.dumpFilteredQueryToIntent(oplogQuery, dump.manager.Oplog(), dump.getResettableOutputBuffer(), oplogDocumentFilter)
	if err == nil {
		log.Logvf(log.Always, "\tdumped %v oplog %v",
			oplogCount, util.Pluralize(int(oplogCount), "entry", "entries"))
	}
	return err
}
