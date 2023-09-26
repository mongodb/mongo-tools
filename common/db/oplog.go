package db

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
)

// ApplyOpsResponse represents the response from an 'applyOps' command.
type ApplyOpsResponse struct {
	Ok     bool   `bson:"ok"`
	ErrMsg string `bson:"errmsg"`
}

// Oplog represents a MongoDB oplog document.
type Oplog struct {
	Timestamp  primitive.Timestamp `bson:"ts"`
	Term       *int64              `bson:"t"`
	Hash       *int64              `bson:"h,omitempty"`
	Version    int                 `bson:"v"`
	Operation  string              `bson:"op"`
	Namespace  string              `bson:"ns"`
	Object     bson.D              `bson:"o"`
	Query      bson.D              `bson:"o2,omitempty"`
	UI         *primitive.Binary   `bson:"ui,omitempty"`
	LSID       bson.Raw            `bson:"lsid,omitempty"`
	TxnNumber  *int64              `bson:"txnNumber,omitempty"`
	PrevOpTime bson.Raw            `bson:"prevOpTime,omitempty"`
}

// OplogTailTime represents two ways of describing the "end" of the oplog at a
// point in time.  The Latest field represents the last visible (storage
// committed) timestamp.  The Restart field represents a (possibly older)
// timestamp that can be used to start tailing or copying the oplog without
// losing parts of transactions in progress.
type OplogTailTime struct {
	Latest  OpTime
	Restart OpTime
}

var zeroTimestamp = primitive.Timestamp{}

// GetOpTimeFromRawOplogEntry looks up the ts (timestamp), t (term), and
// h (hash) fields in a raw oplog entry, and assigns them to an OpTime.
// If the Timestamp can't be found or is an invalid format, it throws an error.
// If the Term or Hash fields can't be found, it returns the OpTime without them.
func GetOpTimeFromRawOplogEntry(rawOplogEntry bson.Raw) (OpTime, error) {
	// Look up the timestamp and store it in an opTime.
	rawTS, err := rawOplogEntry.LookupErr("ts")
	if err != nil {
		return OpTime{}, fmt.Errorf("raw oplog entry had no `ts` field")
	}

	t, i, ok := rawTS.TimestampOK()
	if !ok {
		return OpTime{}, fmt.Errorf("raw oplog entry `ts` field was not a BSON timestamp")
	}

	opTime := OpTime{
		Timestamp: primitive.Timestamp{T: t, I: i},
		Term:      nil,
		Hash:      nil,
	}

	// Look up the term and (if it exists) assign it to the opTime.
	rawTerm, err := rawOplogEntry.LookupErr("t")
	if err == nil {
		term, ok := rawTerm.Int64OK()
		if !ok {
			return OpTime{}, fmt.Errorf("raw oplog entry `t` field was not a BSON int64")
		}
		opTime.Term = &term
	}

	// Look up the hash and (if it exists) assign it to the opTime.
	rawHash, err := rawOplogEntry.LookupErr("h")
	if err == nil {
		hash, ok := rawHash.Int64OK()
		if !ok {
			return OpTime{}, fmt.Errorf("raw oplog entry `h` field  was not a BSON int64")
		}
		opTime.Hash = &hash
	}

	return opTime, nil
}

// GetOplogTailTime constructs an OplogTailTime
func GetOplogTailTime(client *mongo.Client) (OplogTailTime, error) {
	// Check oldest active first to be sure it is less-than-or-equal to the
	// latest visible.
	oldestActive, err := GetOldestActiveTransactionOpTime(client)
	if err != nil {
		return OplogTailTime{}, err
	}
	latestVisible, err := GetLatestVisibleOplogOpTime(client)
	if err != nil {
		return OplogTailTime{}, err
	}
	// No oldest active means the latest visible is the restart time as well.
	if OpTimeIsEmpty(oldestActive) {
		return OplogTailTime{Latest: latestVisible, Restart: latestVisible}, nil
	}
	return OplogTailTime{Latest: latestVisible, Restart: oldestActive}, nil
}

// GetOldestActiveTransactionOpTime returns the oldest active transaction
// optime from the config.transactions table or else a zero-value db.OpTime{}
func GetOldestActiveTransactionOpTime(client *mongo.Client) (OpTime, error) {
	coll := client.Database("config").Collection("transactions", mopts.Collection().SetReadConcern(readconcern.Local()))
	filter := bson.D{{"state", bson.D{{"$in", bson.A{"prepared", "inProgress"}}}}}
	opts := mopts.FindOne().SetSort(bson.D{{"startOpTime", 1}})

	result, err := coll.FindOne(context.Background(), filter, opts).DecodeBytes()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return OpTime{}, nil
		}
		return OpTime{}, fmt.Errorf("config.transactions.findOne error: %v", err)
	}

	startOpTimeRaw := result.Lookup("startOpTime")
	opTime, err := GetOpTimeFromRawOplogEntry(startOpTimeRaw.Value)
	if err != nil {
		return OpTime{}, errors.Wrap(err, "config.transactions error")
	}
	return opTime, nil
}

// GetLatestVisibleOplogOpTime returns the optime of the most recent
// "visible" oplog record. By "visible", we mean that all prior oplog entries
// have been storage-committed. See SERVER-30724 for a more detailed description.
func GetLatestVisibleOplogOpTime(client *mongo.Client) (OpTime, error) {
	latestOpTime, err := GetLatestOplogOpTime(client, bson.D{})
	if err != nil {
		return OpTime{}, err
	}
	// Do a forward scan starting at the last op fetched to ensure that
	// all operations with earlier oplog times have been storage-committed.
	opts := mopts.FindOne().SetOplogReplay(true)
	coll := client.Database("local").Collection("oplog.rs")
	result, err := coll.FindOne(context.Background(), bson.M{"ts": bson.M{"$gte": latestOpTime.Timestamp}}, opts).DecodeBytes()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return OpTime{}, fmt.Errorf("last op was not confirmed. last optime: %+v. confirmation time was not found",
				latestOpTime)
		}
		return OpTime{}, err
	}

	opTime, err := GetOpTimeFromRawOplogEntry(result)
	if err != nil {
		return OpTime{}, errors.Wrap(err, "local.oplog.rs error")
	}

	if !OpTimeEquals(opTime, latestOpTime) {
		return OpTime{}, fmt.Errorf("last op was not confirmed. last optime: %+v. confirmation time: %+v",
			latestOpTime, opTime)
	}
	return latestOpTime, nil
}

// GetLatestOplogOpTime returns the optime of the most recent oplog
// record satisfying the given `query` or a zero-value db.OpTime{} if
// no oplog record matches.  This method does not ensure that all prior oplog
// entries are visible (i.e. have been storage-committed).
func GetLatestOplogOpTime(client *mongo.Client, query interface{}) (OpTime, error) {
	var record Oplog
	opts := mopts.FindOne().SetProjection(bson.M{"ts": 1, "t": 1, "h": 1}).SetSort(bson.D{{"$natural", -1}})
	coll := client.Database("local").Collection("oplog.rs")
	res := coll.FindOne(context.Background(), query, opts)
	if err := res.Err(); err != nil {
		return OpTime{}, err
	}

	if err := res.Decode(&record); err != nil {
		return OpTime{}, err
	}
	return GetOpTimeFromOplogEntry(&record), nil
}
