package mongoproto

import (
	"fmt"
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/mongodb/mongo-tools/common/json"
	"io"
	"time"
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
type ReplyOp struct {
	Header MsgHeader
	mgo.ReplyOp
	Docs    []bson.Raw
	Latency time.Duration
}

func (op *ReplyOp) Meta() OpMetadata {
	return OpMetadata{"", "", ""}
}

func (opr *ReplyOp) String() string {
	if opr == nil {
		return "Reply NIL"
	}
	return fmt.Sprintf("ReplyOp latency:%v reply:[flags:%s, cursorid:%s, first:%s ndocs:%s] docs:%s",
		opr.Latency,
		opr.Flags, opr.CursorId, opr.FirstDoc, opr.ReplyDocs,
		Abbreviate(stringifyReplyDocs(opr.Docs), 256))
}

func (op *ReplyOp) OpCode() OpCode {
	return OpCodeReply
}

func (op *ReplyOp) FromReader(r io.Reader) error {
	var b [20]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return err
	}
	op.Flags = uint32(getInt32(b[:], 0))
	op.CursorId = getInt64(b[:], 4)
	op.FirstDoc = getInt32(b[:], 12)
	op.ReplyDocs = getInt32(b[:], 16)
	op.Docs = []bson.Raw{}

	// read as many docs as we can from the reader
	for {
		docBytes, err := ReadDocument(r)
		if err != nil {
			if err != io.EOF {
				// Broken BSON in reply data. TODO log something here?
				return err
			}
			break
		}
		if len(docBytes) == 0 {
			break
		}
		nextDoc := bson.Raw{}
		err = bson.Unmarshal(docBytes, &nextDoc)
		if err != nil {
			// Unmarshaling []byte to bson.Raw should never ever fail.
			panic("failed to unmarshal []byte to Raw")
		}
		op.Docs = append(op.Docs, nextDoc)
	}

	return nil
}

func (op *ReplyOp) Execute(session *mgo.Session) (*ReplyOp, error) {
	return nil, nil
}

func (replyOp1 *ReplyOp) Equals(otherOp Op) bool {
	return true
}

func stringifyReplyDocs(d []bson.Raw) string {
	if len(d) == 0 {
		return "[empty]"
	}
	docsConverted, err := ConvertBSONValueToJSON(d)
	if err != nil {
		return fmt.Sprintf("ConvertBSONValueToJSON err on reply docs: %v", err)
	}
	asJSON, err := json.Marshal(docsConverted)
	if err != nil {
		return fmt.Sprintf("json marshal err on reply docs: %v", err)
	}
	return string(asJSON)
}
