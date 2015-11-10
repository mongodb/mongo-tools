package mongoproto

import (
	"fmt"
	mgo "github.com/10gen/llmgo"
	"io"
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
}

func (op *ReplyOp) String() string {
	return fmt.Sprintf("op: %v\nflags: %v\ncursorId: %v\nstartingFrom: %v\n numReturned: %v\n", op.OpCode(), op.Flags, op.CursorId, op.FirstDoc, op.ReplyDocs)
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
	return nil
}
func (op *ReplyOp) Execute(session *mgo.Session) (*mgo.ReplyOp, error) {
	return nil, nil
}
func (replyOp1 *ReplyOp) Equals(otherOp Op) bool {
	return true
}
