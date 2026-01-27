package db

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Oplog represents a MongoDB oplog document.
type Oplog struct {
	Timestamp   primitive.Timestamp `bson:"ts"`
	Term        *int64              `bson:"t"`
	Hash        *int64              `bson:"h,omitempty"`
	Version     int                 `bson:"v"`
	Operation   string              `bson:"op"`
	Namespace   string              `bson:"ns"`
	Object      bson.D              `bson:"o"`
	Query       bson.D              `bson:"o2,omitempty"`
	UI          *primitive.Binary   `bson:"ui,omitempty"`
	LSID        bson.Raw            `bson:"lsid,omitempty"`
	TxnNumber   *int64              `bson:"txnNumber,omitempty"`
	PrevOpTime  bson.Raw            `bson:"prevOpTime,omitempty"`
	MultiOpType *int                `bson:"multiOpType,omitempty"`
}
