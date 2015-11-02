package mongoproto

import (
	"io"
	"fmt"
	"encoding/json"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/10gen/llmgo"
	"gopkg.in/mgo.v2/bson"
)

// OpUpdate is used to update a document in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-update
type UpdateOp struct {
	Header MsgHeader
	mgo.UpdateOp
}
func (op *UpdateOp) String() string {
	selectorDoc, err := bsonutil.ConvertBSONValueToJSON(op.Selector)
	if err != nil {
		return fmt.Sprintf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}

	updateDoc, err := bsonutil.ConvertBSONValueToJSON(op.Update)
	if err != nil {
		return fmt.Sprintf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}
	selectorAsJson, err := json.Marshal(selectorDoc)
	if err != nil {
		return fmt.Sprintf("json marshal err: %#v - %v", op, err)
	}
	updateAsJson, err := json.Marshal(updateDoc)
	if err != nil {
		return fmt.Sprintf("json marshal err: %#v - %v", op, err)
	}
	return fmt.Sprintf("OpQuery %v %v %v", op.Collection, string(selectorAsJson), string(updateAsJson))
}

func (op *UpdateOp) OpCode() OpCode {
	return OpCodeUpdate
}


func (op *UpdateOp) FromReader(r io.Reader) error {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:4]); err != nil { // skip ZERO
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Flags = uint32(getInt32(b[:], 4))

	if _, err := io.ReadFull(r, b[4:]); err != nil { // grab the flags
		return err
	}
	op.Collection = string(name)

	selectorAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}

	if err = bson.Unmarshal(selectorAsSlice, op.Selector); err != nil {
		return err
	}

	updateAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}

	if err = bson.Unmarshal(updateAsSlice, op.Update); err != nil {
		return err
	}

	return nil
}

func (op *UpdateOp) Execute(session *mgo.Session) error {
	if err := session.UpdateOp(&op.UpdateOp); err != nil {
		return err
	}

	fmt.Println("Update Op")
	return nil
}