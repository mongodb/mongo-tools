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

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/txn"
	"github.com/mongodb/mongo-tools/common/util"
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

var knownCommands = map[string]bool{
	"renameCollection": true,
	"dropDatabase":     true,
	"applyOps":         true,
	"dbCheck":          true,
	"create":           true,
	"convertToCapped":  true,
	"emptycapped":      true,
	"drop":             true,
	"createIndexes":    true,
	"deleteIndex":      true,
	"deleteIndexes":    true,
	"dropIndex":        true,
	"dropIndexes":      true,
	"collMod":          true,
	"startIndexBuild":  true,
	"abortIndexBuild":  true,
	"commitIndexBuild": true,
}

var errorTimestampBeforeLimit = fmt.Errorf("timestamp before limit")

// shouldIgnoreNamespace returns true if the given namespace should be ignored during applyOps.
func shouldIgnoreNamespace(ns string) bool {
	if strings.HasPrefix(ns, "config.cache.") || ns == "config.system.sessions" || ns == "config.system.indexBuilds" {
		log.Logv(log.Always, "skipping applying the "+ns+" namespace in applyOps")
		return true
	}
	return false
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
	// them referencing the source buffer.
	// We also increase the max BSON size by 16 KiB to accommodate the maximum
	// document size of 16 MiB plus any additional oplog-specific data.
	bsonSource := db.NewBufferlessBSONSource(intent.BSONFile)
	bsonSource.SetMaxBSONSize(db.MaxBSONSize + 16*1024)
	decodedBsonSource := db.NewDecodedBSONSource(bsonSource)
	defer decodedBsonSource.Close()

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
		rawOplogEntry := decodedBsonSource.LoadNext()
		if rawOplogEntry == nil {
			break
		}
		oplogCtx.progressor.Inc(int64(len(rawOplogEntry)))

		entryAsOplog := db.Oplog{}

		err = bson.Unmarshal(rawOplogEntry, &entryAsOplog)
		if err != nil {
			return fmt.Errorf("error reading oplog: %v", err)
		}

		err := restore.HandleOp(oplogCtx, entryAsOplog)
		if err == errorTimestampBeforeLimit {
			break
		}
		if err != nil {
			return err
		}

	}
	if fileNeedsIOBuffer, ok := intent.BSONFile.(intents.FileNeedsIOBuffer); ok {
		fileNeedsIOBuffer.ReleaseIOBuffer()
	}

	log.Logvf(log.Always, "applied %v oplog entries", oplogCtx.totalOps)
	if err := decodedBsonSource.Err(); err != nil {
		return fmt.Errorf("error reading oplog bson input: %v", err)
	}
	return nil

}

func (restore *MongoRestore) HandleOp(oplogCtx *oplogContext, op db.Oplog) error {
	if shouldIgnoreNamespace(op.Namespace) {
		return nil
	}

	if op.Operation == "n" {
		//skip no-ops
		return nil
	}

	if op.Operation == "c" && len(op.Object) > 0 {
		entryName := op.Object[0].Key
		if entryName == "startIndexBuild" || entryName == "abortIndexBuild" {
			log.Logv(log.Always, "skipping applying the oplog entry "+entryName)
			return nil
		}
	}

	if !restore.TimestampBeforeLimit(op.Timestamp) {
		log.Logvf(
			log.DebugLow,
			"timestamp %v is not below limit of %v; ending oplog restoration",
			op.Timestamp,
			restore.oplogLimit,
		)
		return errorTimestampBeforeLimit
	}

	meta, err := txn.NewMeta(op)
	if err != nil {
		return fmt.Errorf("error getting op metadata: %v", err)
	}

	if meta.IsTxn() {
		err := restore.HandleTxnOp(oplogCtx, meta, op)
		if err != nil {
			return fmt.Errorf("error handling transaction oplog entry: %v", err)
		}
	} else {
		err := restore.HandleNonTxnOp(oplogCtx, op)
		if err != nil {
			return fmt.Errorf("error applying oplog: %v", err)
		}
	}

	return nil
}

func (restore *MongoRestore) HandleNonTxnOp(oplogCtx *oplogContext, op db.Oplog) error {
	oplogCtx.totalOps++

	op, err := restore.filterUUIDs(op)
	if err != nil {
		return fmt.Errorf("error filtering UUIDs from oplog: %v", err)
	}

	if op.Operation == "c" {
		if len(op.Object) == 0 {
			return fmt.Errorf("Empty object value for op: %v", op)
		}
		cmdName := op.Object[0].Key

		if !knownCommands[cmdName] {
			return fmt.Errorf("unknown oplog command name %v: %v", cmdName, op)
		}

		ns := strings.Split(op.Namespace, ".")
		dbName := ns[0]

		switch cmdName {
		case "commitIndexBuild":
			// commitIndexBuild was introduced in 4.4, one "commitIndexBuild" command can contain several
			// indexes, we need to convert the command to "createIndexes" command for each single index and apply
			collectionName, indexes := extractIndexDocumentFromCommitIndexBuilds(op)
			if indexes == nil {
				return fmt.Errorf("failed to parse IndexDocument from commitIndexBuild in %s, %v", collectionName, op)
			}

			if restore.OutputOptions.ConvertLegacyIndexes {
				indexes = restore.convertLegacyIndexes(indexes, op.Namespace)
			}

			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}

			restore.indexCatalog.AddIndexes(dbName, collName, indexes)
			return nil

		case "createIndexes":
			// server > 4.4 no longer supports applying createIndexes oplog, we need to convert the oplog to createIndexes command and execute it
			collectionName, index := extractIndexDocumentFromCreateIndexes(op)
			if index.Key == nil {
				return fmt.Errorf("failed to parse IndexDocument from createIndexes in %s, %v", collectionName, op)
			}

			indexes := []*idx.IndexDocument{index}
			if restore.OutputOptions.ConvertLegacyIndexes {
				indexes = restore.convertLegacyIndexes(indexes, op.Namespace)
			}

			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}

			restore.indexCatalog.AddIndexes(dbName, collName, indexes)
			return nil

		case "dropDatabase":
			restore.indexCatalog.DropDatabase(dbName)

		case "drop":
			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}
			restore.indexCatalog.DropCollection(dbName, collName)

		case "applyOps":
			rawOps, ok := op.Object[0].Value.(bson.A)
			if !ok {
				return fmt.Errorf("unknown format for applyOps: %#v", op.Object)
			}

			for _, rawOp := range rawOps {
				bytesOp, err := bson.Marshal(rawOp)
				if err != nil {
					return fmt.Errorf("could not marshal applyOps operation: %v: %v", rawOp, err)
				}
				var nestedOp db.Oplog
				err = bson.Unmarshal(bytesOp, &nestedOp)
				if err != nil {
					return fmt.Errorf("could not unmarshal applyOps command: %v: %v", rawOp, err)
				}

				err = restore.HandleOp(oplogCtx, nestedOp)
				if err != nil {
					return fmt.Errorf("error applying nested op from applyOps: %v", err)
				}
			}

			return nil

		case "deleteIndex", "deleteIndexes", "dropIndex", "dropIndexes":
			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}
			restore.indexCatalog.DeleteIndexes(dbName, collName, op.Object)
			return nil
		case "collMod":
			if restore.serverVersion.GTE(db.Version{4, 1, 11}) {
				_, _ = bsonutil.RemoveKey("noPadding", &op.Object)
				_, _ = bsonutil.RemoveKey("usePowerOf2Sizes", &op.Object)
			}

			indexModValue, found := bsonutil.RemoveKey("index", &op.Object)
			if !found {
				break
			}
			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}
			err := restore.indexCatalog.CollMod(dbName, collName, indexModValue)
			if err != nil {
				return err
			}
			// Don't apply the collMod if the only modification was for an index.
			if len(op.Object) == 1 {
				return nil
			}
		case "create":
			collName, ok := op.Object[0].Value.(string)
			if !ok {
				return fmt.Errorf("could not parse collection name from op: %v", op)
			}
			collation, err := bsonutil.FindSubdocumentByKey("collation", &op.Object)
			if err != nil {
				restore.indexCatalog.SetCollation(dbName, collName, true)
			}
			localeValue, _ := bsonutil.FindValueByKey("locale", &collation)
			if localeValue == "simple" {
				restore.indexCatalog.SetCollation(dbName, collName, true)
			}
		}
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

// extractIndexDocumentFromCommitIndexBuilds extracts the index specs out of  "commitIndexBuild" oplog entry and convert to IndexDocument
// returns collection name and index specs
func extractIndexDocumentFromCommitIndexBuilds(op db.Oplog) (string, []*idx.IndexDocument) {
	collectionName := ""
	for _, elem := range op.Object {
		if elem.Key == "commitIndexBuild" {
			collectionName = elem.Value.(string)
		}
	}
	// We need second iteration to split the indexes into single createIndex command
	for _, elem := range op.Object {
		if elem.Key == "indexes" {
			indexes := elem.Value.(bson.A)
			indexDocuments := make([]*idx.IndexDocument, len(indexes))
			for i, index := range indexes {
				var indexSpec idx.IndexDocument
				indexSpec.Options = bson.M{}
				for _, elem := range index.(bson.D) {
					if elem.Key == "key" {
						indexSpec.Key = elem.Value.(bson.D)
					} else if elem.Key == "partialFilterExpression" {
						indexSpec.PartialFilterExpression = elem.Value.(bson.D)
					} else {
						indexSpec.Options[elem.Key] = elem.Value
					}
				}
				indexDocuments[i] = &indexSpec
			}

			return collectionName, indexDocuments
		}
	}

	return collectionName, nil
}

// extractIndexDocumentFromCommitIndexBuilds extracts the index specs out of  "createIndexes" oplog entry and convert to IndexDocument
// returns collection name and index spec
func extractIndexDocumentFromCreateIndexes(op db.Oplog) (string, *idx.IndexDocument) {
	collectionName := ""
	indexDocument := &idx.IndexDocument{Options: bson.M{}}
	for _, elem := range op.Object {
		if elem.Key == "createIndexes" {
			collectionName = elem.Value.(string)
		} else if elem.Key == "key" {
			indexDocument.Key = elem.Value.(bson.D)
		} else if elem.Key == "partialFilterExpression" {
			indexDocument.PartialFilterExpression = elem.Value.(bson.D)
		} else {
			indexDocument.Options[elem.Key] = elem.Value
		}
	}

	return collectionName, indexDocument
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
