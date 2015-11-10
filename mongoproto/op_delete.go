package mongoproto

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/bsonutil"
)

// OpDelete is used to remove one or more documents from a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-delete

type DeleteOp struct {
	Header MsgHeader
	mgo.DeleteOp
}

func (op *DeleteOp) String() string {
	jsonDoc, err := bsonutil.ConvertBSONValueToJSON(op.Selector)
	if err != nil {
		return fmt.Sprintf("%#v - %v", op, err)
	}
	selectorAsJSON, _ := json.Marshal(jsonDoc)
	return fmt.Sprintf("DeleteOp %v %v", op.Collection, string(selectorAsJSON))
}

func (op *DeleteOp) OpCode() OpCode {
	return OpCodeDelete
}

func (op *DeleteOp) FromReader(r io.Reader) error {
	var b [4]byte
	_, err := io.ReadFull(r, b[:]) //skip ZERO
	if err != nil {
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)
	_, err = io.ReadFull(r, b[:]) //Grab the flags
	if err != nil {
		return err
	}
	op.Flags = uint32(getInt32(b[:], 0))

	selectorAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}
	op.Selector = &bson.D{}
	err = bson.Unmarshal(selectorAsSlice, op.Selector)

	if err != nil {
		return err
	}
	return nil
}

func (op *DeleteOp) Execute(session *mgo.Session) (*mgo.ReplyOp, error) {
	if err := mgo.ExecOpWithoutReply(session, &op.DeleteOp); err != nil {
		return nil, err
	}
	return nil, nil
}

func (deleteOp1 *DeleteOp) Equals(otherOp Op) bool {
	deleteOp2, ok := otherOp.(*DeleteOp)
	if !ok {
		return false
	}
	switch {
	case deleteOp1.Collection != deleteOp2.Collection:
		return false
	case !reflect.DeepEqual(deleteOp1.Selector, deleteOp2.Selector):
		return false
	case deleteOp1.Flags != deleteOp2.Flags:
		return false
	}
	return true
}

