package mongoproto

import "gopkg.in/mgo.v2/bson"

const (
	OpDeleteSingleRemove OpDeleteFlags = 1 << iota
)

type OpDeleteFlags int32

// OpDelete is used to remove one or more documents from a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-delete
type OpDelete struct {
	Header             MsgHeader
	FullCollectionName string // "dbname.collectionname"
	Flags              OpDeleteFlags
	Selector           *bson.D // the query to select the document(s)
}
