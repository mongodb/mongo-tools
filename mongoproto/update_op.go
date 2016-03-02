package mongoproto

import (
	"encoding/json"
	"fmt"
	"github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"io"
	"reflect"
)

// OpUpdate is used to update a document in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-update
type UpdateOp struct {
	Header MsgHeader
	mgo.UpdateOp
}

func (op *UpdateOp) Meta() OpMetadata {
	return OpMetadata{"update", op.Collection, ""}
}

func (op *UpdateOp) String() string {
	selectorString, updateString, err := op.getOpBodyString()
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return fmt.Sprintf("OpQuery %v %v %v", op.Collection, selectorString, updateString)
}

func (op *UpdateOp) getOpBodyString() (string, string, error) {
	selectorDoc, err := ConvertBSONValueToJSON(op.Selector)
	if err != nil {
		return "", "", fmt.Errorf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}

	updateDoc, err := ConvertBSONValueToJSON(op.Update)
	if err != nil {
		return "", "", fmt.Errorf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}
	selectorAsJson, err := json.Marshal(selectorDoc)
	if err != nil {
		return "", "", fmt.Errorf("json marshal err: %#v - %v", op, err)
	}
	updateAsJson, err := json.Marshal(updateDoc)
	if err != nil {
		return "", "", fmt.Errorf("json marshal err: %#v - %v", op, err)
	}
	return string(selectorAsJson), string(updateAsJson), nil
}
func (op *UpdateOp) Abbreviated(chars int) string {
	selectorString, updateString, err := op.getOpBodyString()
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return fmt.Sprintf("OpQuery %v %v %v", op.Collection, Abbreviate(selectorString, chars), Abbreviate(updateString, chars))
}

func (op *UpdateOp) OpCode() OpCode {
	return OpCodeUpdate
}

func (op *UpdateOp) FromReader(r io.Reader) error {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil { // skip ZERO
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)

	if _, err := io.ReadFull(r, b[:]); err != nil { // grab the flags
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

	updateAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}
	op.Update = &bson.D{}
	err = bson.Unmarshal(updateAsSlice, op.Update)
	if err != nil {
		return err
	}

	return nil
}

func (op *UpdateOp) Execute(session *mgo.Session) (*ReplyOp, error) {
	if err := mgo.ExecOpWithoutReply(session, &op.UpdateOp); err != nil {
		return nil, err
	}
	return nil, nil
}

func (updateOp1 *UpdateOp) Equals(otherOp Op) bool {
	updateOp2, ok := otherOp.(*UpdateOp)
	if !ok {
		return false
	}
	switch {
	case updateOp1.Collection != updateOp2.Collection:
		return false
	case reflect.DeepEqual(updateOp1.Selector, updateOp2.Selector):
		return false
	case reflect.DeepEqual(updateOp1.Update, updateOp2.Update):
		return false
	case updateOp1.Flags != updateOp2.Flags:
		return false
	}
	return true
}
