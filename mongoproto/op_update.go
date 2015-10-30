package mongoproto

import (
	"github.com/10gen/llmgo"
)

// OpUpdate is used to update a document in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-update
type UpdateOp struct {
	Header MsgHeader
	mgo.UpdateOp
}
