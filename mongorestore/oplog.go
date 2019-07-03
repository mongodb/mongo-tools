// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/intents"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/progress"
	"github.com/mongodb/mongo-tools-common/txn"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// oplogMaxCommandSize sets the maximum size for multiple buffered ops in the
// applyOps command. This is to prevent pathological cases where the array overhead
// of many small operations can overflow the maximum command size.
// Note that ops > 8MB will still be buffered, just as single elements.
const oplogMaxCommandSize = 1024 * 1024 * 8

type oplogContext struct {
	progressor *progress.CountProgressor
	session    *mongo.Client
	totalOps   int
	txnBuffer  *txn.Buffer
}

// RestoreOplog attempts to restore a MongoDB oplog.
func (restore *MongoRestore) RestoreOplog() error {
	log.Logv(log.Always, "replaying oplog")
	intent := restore.manager.Oplog()
	if intent == nil {
		// this should not be reached
		log.Logv(log.Always, "no oplog file provided, skipping oplog application")
		return nil
	}
	if err := intent.BSONFile.Open(); err != nil {
		return err
	}
	if fileNeedsIOBuffer, ok := intent.BSONFile.(intents.FileNeedsIOBuffer); ok {
		fileNeedsIOBuffer.TakeIOBuffer(make([]byte, db.MaxBSONSize))
	}
	defer intent.BSONFile.Close()
	// NewBufferlessBSONSource reads each bson document into its own buffer
	// because bson.Unmarshal currently can't unmarshal binary types without
	// them referencing the source buffer
	bsonSource := db.NewDecodedBSONSource(db.NewBufferlessBSONSource(intent.BSONFile))
	defer bsonSource.Close()

	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}

	oplogCtx := &oplogContext{
		progressor: progress.NewCounter(intent.BSONSize),
		txnBuffer:  txn.NewBuffer(),
		session:    session,
	}
	defer oplogCtx.txnBuffer.Stop()

	if restore.ProgressManager != nil {
		restore.ProgressManager.Attach("oplog", oplogCtx.progressor)
		defer restore.ProgressManager.Detach("oplog")
	}

	for {
		rawOplogEntry := bsonSource.LoadNext()
		if rawOplogEntry == nil {
			break
		}
		oplogCtx.progressor.Inc(int64(len(rawOplogEntry)))

		entryAsOplog := db.Oplog{}
		err = bson.Unmarshal(rawOplogEntry, &entryAsOplog)
		if err != nil {
			return fmt.Errorf("error reading oplog: %v", err)
		}
		if entryAsOplog.Operation == "n" {
			//skip no-ops
			continue
		}
		if !restore.TimestampBeforeLimit(entryAsOplog.Timestamp) {
			log.Logvf(
				log.DebugLow,
				"timestamp %v is not below limit of %v; ending oplog restoration",
				entryAsOplog.Timestamp,
				restore.oplogLimit,
			)
			break
		}

		meta, err := txn.NewMeta(entryAsOplog)
		if err != nil {
			return fmt.Errorf("error getting op metadata: %v", err)
		}

		if meta.IsTxn() {
			err := restore.HandleTxnOp(oplogCtx, meta, entryAsOplog)
			if err != nil {
				return fmt.Errorf("error handling transaction oplog entry: %v", err)
			}
		} else {
			err := restore.HandleNonTxnOp(oplogCtx, entryAsOplog)
			if err != nil {
				return fmt.Errorf("error applying oplog: %v", err)
			}
		}

	}
	if fileNeedsIOBuffer, ok := intent.BSONFile.(intents.FileNeedsIOBuffer); ok {
		fileNeedsIOBuffer.ReleaseIOBuffer()
	}

	log.Logvf(log.Always, "applied %v oplog entries", oplogCtx.totalOps)
	if err := bsonSource.Err(); err != nil {
		return fmt.Errorf("error reading oplog bson input: %v", err)
	}
	return nil

}

func (restore *MongoRestore) HandleNonTxnOp(oplogCtx *oplogContext, op db.Oplog) error {
	oplogCtx.totalOps++

	op, err := restore.filterUUIDs(op)
	if err != nil {
		return fmt.Errorf("error filtering UUIDs from oplog: %v", err)
	}

	return restore.ApplyOps(oplogCtx.session, []interface{}{op})
}

func (restore *MongoRestore) HandleTxnOp(oplogCtx *oplogContext, meta txn.Meta, op db.Oplog) error {

	err := oplogCtx.txnBuffer.AddOp(meta, op)
	if err != nil {
		return fmt.Errorf("error buffering transaction oplog entry: %v", err)
	}

	if meta.IsAbort() {
		err := oplogCtx.txnBuffer.PurgeTxn(meta)
		if err != nil {
			return fmt.Errorf("error cleaning up transaction buffer on abort: %v", err)
		}
		return nil
	}

	if !meta.IsCommit() {
		return nil
	}

	// From here, we're applying transaction entries
	ops, errs := oplogCtx.txnBuffer.GetTxnStream(meta)

Loop:
	for {
		select {
		case o, ok := <-ops:
			if !ok {
				break Loop
			}
			err = restore.HandleNonTxnOp(oplogCtx, o)
			if err != nil {
				return fmt.Errorf("error applying transaction op: %v", err)
			}
		case err := <-errs:
			if err != nil {
				return fmt.Errorf("error replaying transaction: %v", err)
			}
			break Loop
		}
	}

	err = oplogCtx.txnBuffer.PurgeTxn(meta)
	if err != nil {
		return fmt.Errorf("error cleaning up transaction buffer: %v", err)
	}

	return nil
}

// ApplyOps is a wrapper for the applyOps database command, we pass in
// a session to avoid opening a new connection for a few inserts at a time.
func (restore *MongoRestore) ApplyOps(session *mongo.Client, entries []interface{}) error {
	singleRes := session.Database("admin").RunCommand(nil, bson.D{{"applyOps", entries}})
	if err := singleRes.Err(); err != nil {
		return fmt.Errorf("applyOps: %v", err)
	}
	res := bson.M{}
	singleRes.Decode(&res)
	if util.IsFalsy(res["ok"]) {
		return fmt.Errorf("applyOps command: %v", res["errmsg"])
	}

	return nil
}

// TimestampBeforeLimit returns true if the given timestamp is allowed to be
// applied to mongorestore's target database.
func (restore *MongoRestore) TimestampBeforeLimit(ts primitive.Timestamp) bool {
	if restore.oplogLimit.T == 0 && restore.oplogLimit.I == 0 {
		// always valid if there is no --oplogLimit set
		return true
	}
	return util.TimestampGreaterThan(restore.oplogLimit, ts)
}

// ParseTimestampFlag takes in a string the form of <time_t>:<ordinal>,
// where <time_t> is the seconds since the UNIX epoch, and <ordinal> represents
// a counter of operations in the oplog that occurred in the specified second.
// It parses this timestamp string and returns a bson.MongoTimestamp type.
func ParseTimestampFlag(ts string) (primitive.Timestamp, error) {
	var seconds, increment int
	timestampFields := strings.Split(ts, ":")
	if len(timestampFields) > 2 {
		return primitive.Timestamp{}, fmt.Errorf("too many : characters")
	}

	seconds, err := strconv.Atoi(timestampFields[0])
	if err != nil {
		return primitive.Timestamp{}, fmt.Errorf("error parsing timestamp seconds: %v", err)
	}

	// parse the increment field if it exists
	if len(timestampFields) == 2 {
		if len(timestampFields[1]) > 0 {
			increment, err = strconv.Atoi(timestampFields[1])
			if err != nil {
				return primitive.Timestamp{}, fmt.Errorf("error parsing timestamp increment: %v", err)
			}
		} else {
			// handle the case where the user writes "<time_t>:" with no ordinal
			increment = 0
		}
	}

	return primitive.Timestamp{T: uint32(seconds), I: uint32(increment)}, nil
}

// Server versions 3.6.0-3.6.8 and 4.0.0-4.0.2 require a 'ui' field
// in the createIndexes command.
func (restore *MongoRestore) needsCreateIndexWorkaround() bool {
	sv := restore.serverVersion
	if (sv.GTE(db.Version{3, 6, 0}) && sv.LTE(db.Version{3, 6, 8})) ||
		(sv.GTE(db.Version{4, 0, 0}) && sv.LTE(db.Version{4, 0, 2})) {
		return true
	}
	return false
}

// filterUUIDs removes 'ui' entries from ops, including nested applyOps ops.
// It also modifies ops that rely on 'ui'.
func (restore *MongoRestore) filterUUIDs(op db.Oplog) (db.Oplog, error) {
	// Remove UUIDs from oplog entries
	if !restore.OutputOptions.PreserveUUID {
		op.UI = nil

		// The createIndexes oplog command requires 'ui' for some server versions, so
		// in that case we fall back to an old-style system.indexes insert.
		if op.Operation == "c" && op.Object[0].Key == "createIndexes" && restore.needsCreateIndexWorkaround() {
			return convertCreateIndexToIndexInsert(op)
		}
	}

	// Check for and filter nested applyOps ops
	if op.Operation == "c" && isApplyOpsCmd(op.Object) {
		filtered, err := restore.newFilteredApplyOps(op.Object)
		if err != nil {
			return db.Oplog{}, err
		}
		op.Object = filtered
	}

	return op, nil
}

// convertCreateIndexToIndexInsert converts from new-style create indexes
// command to old style special index insert.
func convertCreateIndexToIndexInsert(op db.Oplog) (db.Oplog, error) {
	dbName, _ := util.SplitNamespace(op.Namespace)

	cmdValue := op.Object[0].Value
	collName, ok := cmdValue.(string)
	if !ok {
		return db.Oplog{}, fmt.Errorf("unknown format for createIndexes")
	}

	indexSpec := op.Object[1:]
	if len(indexSpec) < 3 {
		return db.Oplog{}, fmt.Errorf("unknown format for createIndexes, index spec " +
			"must have at least \"v\", \"key\", and \"name\" fields")
	}

	// createIndexes does not include the "ns" field but index inserts
	// do. Add it as the third field, after "v", "key", and "name".
	ns := bson.D{{"ns", fmt.Sprintf("%s.%s", dbName, collName)}}
	indexSpec = append(indexSpec[:3], append(ns, indexSpec[3:]...)...)
	op.Object = indexSpec
	op.Namespace = fmt.Sprintf("%s.system.indexes", dbName)
	op.Operation = "i"

	return op, nil
}

// isApplyOpsCmd returns true if a document seems to be an applyOps command.
func isApplyOpsCmd(cmd bson.D) bool {
	for _, v := range cmd {
		if v.Key == "applyOps" {
			return true
		}
	}
	return false
}

// newFilteredApplyOps iterates over nested ops in an applyOps document and
// returns a new applyOps document that omits the 'ui' field from nested ops.
func (restore *MongoRestore) newFilteredApplyOps(cmd bson.D) (bson.D, error) {
	ops, err := unwrapNestedApplyOps(cmd)
	if err != nil {
		return nil, err
	}

	filtered := make([]db.Oplog, len(ops))
	for i, v := range ops {
		filtered[i], err = restore.filterUUIDs(v)
		if err != nil {
			return nil, err
		}
	}

	doc, err := wrapNestedApplyOps(filtered)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// nestedApplyOps models an applyOps command document
type nestedApplyOps struct {
	ApplyOps []db.Oplog `bson:"applyOps"`
}

// unwrapNestedApplyOps converts a bson.D to a typed data structure.
// Unfortunately, we're forced to convert by marshaling to bytes and
// unmarshalling.
func unwrapNestedApplyOps(doc bson.D) ([]db.Oplog, error) {
	// Doc to bytes
	bs, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot remarshal nested applyOps: %s", err)
	}

	// Bytes to typed data
	var cmd nestedApplyOps
	err = bson.Unmarshal(bs, &cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot unwrap nested applyOps: %s", err)
	}

	return cmd.ApplyOps, nil
}

// wrapNestedApplyOps converts a typed data structure to a bson.D.
// Unfortunately, we're forced to convert by marshaling to bytes and
// unmarshalling.
func wrapNestedApplyOps(ops []db.Oplog) (bson.D, error) {
	cmd := &nestedApplyOps{ApplyOps: ops}

	// Typed data to bytes
	raw, err := bson.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("cannot rewrap nested applyOps op: %s", err)
	}

	// Bytes to doc
	var doc bson.D
	err = bson.Unmarshal(raw, &doc)
	if err != nil {
		return nil, fmt.Errorf("cannot reunmarshal nested applyOps op: %s", err)
	}

	return doc, nil
}
