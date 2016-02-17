package mongoproto

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
)

// OpInsert is used to insert one or more documents into a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-insert

type InsertOp struct {
	Header MsgHeader
	mgo.InsertOp
}

func (op *InsertOp) Meta() OpMetadata {
	return OpMetadata{"insert", op.Collection, ""}
}

func (op *InsertOp) OpCode() OpCode {
	return OpCodeInsert
}

func (op *InsertOp) String() string {
	docs := make([]string, 0, len(op.Documents))
	for _, d := range op.Documents {
		jsonDoc, err := ConvertBSONValueToJSON(d)
		if err != nil {
			return fmt.Sprintf("%#v - %v", op, err)
		}
		asJSON, _ := json.Marshal(jsonDoc)
		docs = append(docs, string(asJSON))
	}
	return fmt.Sprintf("InsertOp %v %v", op.Collection, docs)
}

func (op *InsertOp) FromReader(r io.Reader) error {
	var b [4]byte
	_, err := io.ReadFull(r, b[:])
	if err != nil {
		return err
	}
	op.Flags = uint32(getInt32(b[:], 0))
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)
	op.Documents = make([]interface{}, 0)

	docLen := 0
	for len(name)+1+4+docLen < int(op.Header.MessageLength)-MsgHeaderLen {
		docAsSlice, err := ReadDocument(r)
		doc := &bson.D{}
		err = bson.Unmarshal(docAsSlice, doc)
		if err != nil {
			return err
		}
		docLen += len(docAsSlice)
		op.Documents = append(op.Documents, doc)
	}
	return nil
}

func (op *InsertOp) Execute(session *mgo.Session) (*ReplyOp, error) {
	session.SetSocketTimeout(0)
	if err := mgo.ExecOpWithoutReply(session, &op.InsertOp); err != nil {
		return nil, err
	}

	return nil, nil
}

func (insertOp1 *InsertOp) Equals(otherOp Op) bool {
	insertOp2, ok := otherOp.(*InsertOp)
	if !ok {
		return false
	}
	switch {
	case insertOp1.Collection != insertOp2.Collection:
		return false
	case reflect.DeepEqual(insertOp1.Documents, insertOp2.Documents):
		return false
	case insertOp1.Flags != insertOp2.Flags:
		return false
	}
	return true
}
