package mongoproto

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	OpInsertContinueOnError OpInsertFlags = 1 << iota
)

type OpInsertFlags int32

// OpInsert is used to insert one or more documents into a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-insert
type OpInsert struct {
	Header             MsgHeader
	Flags              OpInsertFlags
	FullCollectionName string   // "dbname.collectionname"
	Documents          [][]byte // one or more documents to insert into the collection
}

func (op *OpInsert) OpCode() OpCode {
	return OpCodeInsert
}

func (op *OpInsert) String() string {
	docs := make([]string, 0, len(op.Documents))
	var doc interface{}
	for _, d := range op.Documents {
		_ = bson.Unmarshal(d, &doc)
		jsonDoc, err := bsonutil.ConvertBSONValueToJSON(doc)
		if err != nil {
			return fmt.Sprintf("%#v - %v", op, err)
		}
		asJSON, _ := json.Marshal(jsonDoc)
		docs = append(docs, string(asJSON))
	}
	return fmt.Sprintf("OpInsert %v %v", op.FullCollectionName, docs)
}

func (op *OpInsert) FromReader(r io.Reader) error {
	var b [4]byte
	_, err := io.ReadFull(r, b[:])
	if err != nil {
		return err
	}
	op.Flags = OpInsertFlags(getInt32(b[:], 0))
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.FullCollectionName = string(name)
	op.Documents = make([][]byte, 0)

	docLen := 0
	for len(name)+1+4+docLen < int(op.Header.MessageLength)-MsgHeaderLen {
		doc, err := ReadDocument(r)
		if err != nil {
			return err
		}
		docLen += len(doc)
		op.Documents = append(op.Documents, doc)
	}
	return nil
}

func (op *OpInsert) toWire() []byte {
	return nil
}

func (op *OpInsert) Execute(session *mgo.Session) error {
	return nil
}
