package mongoproto

import (
	"fmt"
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"io"
)

// OpGetMore is used to query the database for documents in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-get-more
type GetMoreOp struct {
	Header MsgHeader
	mgo.GetMoreOp
}

func (op *GetMoreOp) OpCode() OpCode {
	return OpCodeGetMore
}

func (op *GetMoreOp) FromReader(r io.Reader) error {
	var b [12]byte
	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)
	if _, err := io.ReadFull(r, b[:12]); err != nil {
		return err
	}
	op.Limit = getInt32(b[:], 0)
	op.CursorId = getInt64(b[:], 4)
	return nil
}

func (op *GetMoreOp) Execute(session *mgo.Session) (*mgo.ReplyOp, error) {
	// XXX don't actually use op.CursorID, but look up the translated cursor id from op.CursorID
	data, reply, err := mgo.ExecOpWithReply(session, &op.GetMoreOp)

	dataDoc := bson.M{}
	for _, d := range data {
		err = bson.Unmarshal(d, dataDoc)
		if err != nil {
			return nil, err
		}
		fmt.Printf("data %#v\n", dataDoc)
		fmt.Printf("reply %#v\n", reply)
	}

	return reply, nil
}

func (getMoreOp1 *GetMoreOp) Equals(otherOp Op) bool {
	getMoreOp2, ok := otherOp.(*GetMoreOp)
	if !ok {
		return false
	}
	switch {
	case getMoreOp1.Collection != getMoreOp2.Collection:
		return false
	case getMoreOp1.Limit != getMoreOp2.Limit:
		return false
	}
	//currently doesn't compare cursorID's, not totally sure what to do about that just yet
	return true
}
