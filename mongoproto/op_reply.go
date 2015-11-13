package mongoproto

import (
	"io"
	mgo "github.com/10gen/llmgo"
)

// OpReply is sent by the database in response to an OpQuery or OpGetMore message.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-reply
type ReplyOp struct {
	Header         MsgHeader
	mgo.ReplyOp
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