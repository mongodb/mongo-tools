// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/failpoint"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/util"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
)

// determineOplogCollectionName uses a command to infer
// the name of the oplog collection in the connected db.
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

// getOplogCurrentTime returns the most recent oplog entry.
func (dump *MongoDump) getCurrentOplogTime() (bson.Timestamp, error) {
	mostRecentOplogEntry := db.Oplog{}
	var tempBSON bson.Raw

	err := dump.SessionProvider.FindOne(
		"local",
		dump.oplogCollection,
		0,
		nil,
		&bson.M{"$natural": -1},
		&tempBSON,
		0,
	)
	if err != nil {
		return bson.Timestamp{}, fmt.Errorf("error getting recent oplog entry: %v", err)
	}
	err = bson.Unmarshal(tempBSON, &mostRecentOplogEntry)
	if err != nil {
		return bson.Timestamp{}, err
	}
	return mostRecentOplogEntry.Timestamp, nil
}

// getOplogCopyStartTime returns either the oldest active transaction timestamp or the
// current oplog time if there are no active transactions.
func (dump *MongoDump) getOplogCopyStartTime() (bson.Timestamp, error) {
	client, err := dump.SessionProvider.GetSession()
	if err != nil {
		return bson.Timestamp{}, fmt.Errorf("error getting client: %v", err)
	}

	coll := client.Database("config").
		Collection("transactions", mopt.Collection().SetReadConcern(readconcern.Local()))
	filter := bson.D{{"state", bson.D{{"$in", bson.A{"prepared", "inProgress"}}}}}
	opts := mopt.FindOne().SetSort(bson.D{{"startOpTime", 1}})

	var result bson.Raw
	res := coll.FindOne(context.Background(), filter, opts)
	err = res.Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return dump.getCurrentOplogTime()
		}
		return bson.Timestamp{}, fmt.Errorf("config.transactions.findOne error: %v", err)
	}

	rawTS, err := result.LookupErr("startOpTime", "ts")
	if err != nil {
		return bson.Timestamp{}, fmt.Errorf(
			"config.transactions row had no startOpTime.ts field",
		)
	}

	t, i, ok := rawTS.TimestampOK()
	if !ok {
		return bson.Timestamp{}, fmt.Errorf(
			"config.transactions startOpTime.ts was not a BSON timestamp",
		)
	}

	return bson.Timestamp{T: t, I: i}, nil
}

// checkOplogTimestampExists checks to make sure the oplog hasn't rolled over
// since mongodump started. It does this by checking the oldest oplog entry
// still in the database and making sure it happened at or before the timestamp
// captured at the start of the dump.
func (dump *MongoDump) checkOplogTimestampExists(ts bson.Timestamp) (bool, error) {
	oldestOplogEntry, err := dump.findOldestOplogEntry()
	if err != nil {
		return false, err
	}

	log.Logvf(log.DebugHigh, "oldest oplog entry has timestamp %v", oldestOplogEntry.Timestamp)
	if util.TimestampGreaterThan(oldestOplogEntry.Timestamp, ts) {
		log.Logvf(log.Info, "oldest oplog entry of timestamp %v is newer than %v",
			oldestOplogEntry.Timestamp, ts)
		return false, nil
	}
	return true, nil
}

// cappedPositionLostErrCode is the server's CappedPositionLost error, returned
// when a collection scan loses its place because the records it was reading
// were deleted out from under it.
const cappedPositionLostErrCode = 136

// oplogReadAttempts bounds how many times findOldestOplogEntry retries a read
// that lost its position.
const oplogReadAttempts = 4

// oplogReadRetryDelay is the wait before the first retry, doubled for each
// retry after that. A single truncation pass has been seen to run for over a
// second, so the delays have to add up to more than that: otherwise every
// attempt lands inside the same pass that killed the first read. The delays
// also can't grow without bound, because each one widens the window in which
// truncation can legitimately advance past oplogStart and turn a good dump
// into a reported overflow.
const oplogReadRetryDelay = 250 * time.Millisecond

// findOldestOplogEntry returns the oldest entry still in the oplog.
//
// Scanning the front of the oplog in $natural order races the server's oplog
// truncation: on a busy oplog the entries being read can be deleted mid-scan,
// which fails the whole read with CappedPositionLost even though the scan had
// already found its one document. That says nothing about whether the dump's
// starting timestamp survived, so such a read is retried rather than reported.
func (dump *MongoDump) findOldestOplogEntry() (db.Oplog, error) {
	var lastErr error
	wait := oplogReadRetryDelay
	for attempt := range oplogReadAttempts {
		if attempt > 0 {
			time.Sleep(wait)
			wait *= 2
		}

		var tempBSON bson.Raw

		lastErr = failOplogCheckReadFailpoint()
		if lastErr == nil {
			lastErr = dump.SessionProvider.FindOne(
				"local",
				dump.oplogCollection,
				0,
				nil,
				&bson.M{"$natural": 1},
				&tempBSON,
				0,
			)
		}

		if lastErr == nil {
			oldestOplogEntry := db.Oplog{}
			if err := bson.Unmarshal(tempBSON, &oldestOplogEntry); err != nil {
				return db.Oplog{}, err
			}
			return oldestOplogEntry, nil
		}

		if !isCappedPositionLost(lastErr) {
			return db.Oplog{}, fmt.Errorf("unable to read entry from oplog: %w", lastErr)
		}

		log.Logvf(log.DebugLow, "oplog read lost its position, retrying: %v", lastErr)
	}

	return db.Oplog{}, fmt.Errorf(
		"unable to read entry from oplog after %d attempts: %w",
		oplogReadAttempts,
		lastErr,
	)
}

// isCappedPositionLost reports whether err is the server saying that a
// collection scan lost its place because the records it was reading were
// deleted while it was reading them.
func isCappedPositionLost(err error) bool {
	var serverErr mongo.ServerError
	return errors.As(err, &serverErr) && serverErr.HasErrorCode(cappedPositionLostErrCode)
}

// failOplogCheckReadFailpoint returns a synthetic CappedPositionLost error the
// first time it is called with the FailOplogCheckRead failpoint enabled, so
// that tests can exercise the retry in findOldestOplogEntry.
func failOplogCheckReadFailpoint() error {
	fp, ok := failpoint.DefaultManager.Get(failpoint.FailOplogCheckRead)
	if !ok || !fp.FireOnce() {
		return nil
	}

	return mongo.CommandError{
		Code: cappedPositionLostErrCode,
		Name: "CappedPositionLost",
		Message: "CollectionScan died due to position in capped collection being deleted" +
			" (injected by the FailOplogCheckRead failpoint)",
	}
}

func oplogDocumentValidator(in []byte) error {
	raw := bson.Raw(in)

	var nsStr string
	var ok bool

	nsVal, err := raw.LookupErr("ns")

	if err == nil {
		nsStr, ok = nsVal.StringValueOK()
	}

	if ok && nsStr == "admin.system.version" {
		return fmt.Errorf("cannot dump with oplog if admin.system.version is modified by %v", raw)
	}

	if _, err := raw.LookupErr("o", "renameCollection"); err == nil {
		return fmt.Errorf("cannot dump with oplog while renames occur")
	}

	if _, err := raw.LookupErr("o", "importCollection"); err == nil {
		return fmt.Errorf("cannot dump with oplog while importCollection occurs")
	}

	// This entry is emitted by setFCV on 9.0+ when a timeseries collection is converted
	// between its viewful and viewless forms (SERVER-114505). mongorestore cannot replay it,
	// so an oplog containing it is not restorable; fail here rather than at restore time.
	if _, err := raw.LookupErr("o", "upgradeDowngradeViewlessTimeseries"); err == nil {
		return fmt.Errorf(
			"cannot dump with oplog while a timeseries format conversion caused by an FCV " +
				"change between 8.x and 9.0 occurs",
		)
	}

	if ok {
		dbName := strings.SplitN(nsStr, ".", 2)[0]

		if dbName == "config" {
			if collNameRaw, err := raw.LookupErr("o", "create"); err == nil {
				collName, ok := collNameRaw.StringValueOK()
				if ok && isReshardingCollection(collName) {
					return fmt.Errorf("cannot dump with oplog while resharding")
				}
			}
		}
	}

	return nil
}

// DumpOplogBetweenTimestamps takes two timestamps and writer and dumps all oplog
// entries between the given timestamp to the writer. Returns any errors that occur.
func (dump *MongoDump) DumpOplogBetweenTimestamps(start, end bson.Timestamp) error {
	session, err := dump.SessionProvider.GetSession()
	if err != nil {
		return err
	}
	queryObj := bson.M{"$and": []bson.M{
		{"ts": bson.M{"$gte": start}},
		{"ts": bson.M{"$lte": end}},
	}}
	oplogQuery := &db.DeferredQuery{
		Coll:      session.Database("local").Collection(dump.oplogCollection),
		Filter:    queryObj,
		LogReplay: true,
	}
	oplogCount, err := dump.dumpValidatedQueryToIntent(
		oplogQuery,
		dump.manager.Oplog(),
		dump.getResettableOutputBuffer(),
		oplogDocumentValidator,
	)
	if err == nil {
		log.Logvf(log.Always, "\tdumped %v oplog %v",
			oplogCount, util.Pluralize(int(oplogCount), "entry", "entries"))
	}
	return err
}
