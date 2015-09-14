package mongoproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"gopkg.in/mgo.v2/bson"
)

const (
	OpReplyCursorNotFound   OpReplyFlags = 1 << iota // Set when getMore is called but the cursor id is not valid at the server. Returned with zero results.
	OpReplyQueryFailure                              // Set when query failed. Results consist of one document containing an “$err” field describing the failure.
	OpReplyShardConfigStale                          //Drivers should ignore this. Only mongos will ever see this set, in which case, it needs to update config from the server.
	OpReplyAwaitCapable                              //Set when the server supports the AwaitData Query option. If it doesn’t, a client should sleep a little between getMore’s of a Tailable cursor. Mongod version 1.6 supports AwaitData and thus always sets AwaitCapable.
)

type OpReplyFlags int32

// OpReply is sent by the database in response to an OpQuery or OpGetMore message.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-reply
type OpReply struct {
	Header         MsgHeader
	Message        string
	Flags          OpReplyFlags
	CursorID       int64    // cursor id if client needs to do get more's
	StartingFrom   int32    // where in the cursor this reply is starting
	NumberReturned int32    // number of documents in the reply
	Documents      [][]byte // documents
}

func (op *OpReply) String() string {
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
	return fmt.Sprintf("OpReply %v %v", op.Message, docs)
}

func (op *OpReply) OpCode() OpCode {
	return OpCodeReply
}

// XXX make a version of this that doesn't read the bodies
func (op *OpReply) FromReader(r io.Reader) error {
	var b [20]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return err
	}

	op.Flags = OpReplyFlags(getInt32(b[:], 0))
	op.CursorID = getInt64(b[:], 4)
	op.StartingFrom = getInt32(b[:], 12)
	op.NumberReturned = getInt32(b[:], 16)
	for i := int32(0); i < op.NumberReturned; i++ {
		doc, err := ReadDocument(r)
		if err != nil {
			return err
		}
		op.Documents = append(op.Documents, doc)
	}
	return nil
}

func (op *OpReply) fromWire(b []byte) {
	if len(b) < 20 {
		return
	}
	op.Flags = OpReplyFlags(getInt32(b, 0))
	op.CursorID = getInt64(b, 4)
	op.StartingFrom = getInt32(b, 12)
	op.NumberReturned = getInt32(b, 16)

	offset := 20
	for i := int32(0); i < op.NumberReturned; i++ {
		doc, err := ReadDocument(bytes.NewReader(b[offset:]))
		if err != nil {
			// TODO(tmc) probably should return an error from fromWire
			log.Println("doc err:", err, len(b[offset:]))
			break
		}
		op.Documents = append(op.Documents, doc)
		offset += len(doc)
	}
}

func (op *OpReply) toWire() []byte {
	return nil
}
