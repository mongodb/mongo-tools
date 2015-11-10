package mongoproto

import (
	"encoding/json"
	"fmt"
	"io"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/bsonutil"
)

// OpInsert is used to insert one or more documents into a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-insert

type InsertOp struct {
	Header MsgHeader
	mgo.InsertOp
}

func (op *InsertOp) OpCode() OpCode {
	return OpCodeInsert
}

func (op *InsertOp) String() string {
	docs := make([]string, 0, len(op.Documents))
	for _, d := range op.Documents {
		jsonDoc, err := bsonutil.ConvertBSONValueToJSON(d)
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
		var doc interface{}
		err = bson.Unmarshal(docAsSlice, doc)
		if err != nil {
			return err
		}
		docLen += len(docAsSlice)
		op.Documents = append(op.Documents, doc)
	}
	return nil
}

func (op *InsertOp) Execute(session *mgo.Session) error {
	if err := session.ExecOpWithoutReply(&op.InsertOp); err != nil {
		return err
	}

	fmt.Println("Insert Op")
	return nil
}
