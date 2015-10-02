package mongoproto

import "gopkg.in/mgo.v2/bson"

const (
	OpUpdateUpsert OpUpdateFlags = 1 << iota
	OpUpdateMuli
)

type OpUpdateFlags int32

// OpUpdate is used to update a document in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-update
type OpUpdate struct {
	Header             MsgHeader
	FullCollectionName string // "dbname.collectionname"
	Flags              OpUpdateFlags
	Selector           *bson.D // the query to select the document
	Update             *bson.D // specification of the update to perform
}
