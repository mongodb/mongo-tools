package mongoproto

import(
	mgo "github.com/10gen/llmgo"
)

// OpDelete is used to remove one or more documents from a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-delete

type DeleteOp struct {
	mgo.DeleteOp
}