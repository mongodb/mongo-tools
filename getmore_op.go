package mongotape

import (
	"fmt"
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"io"
	"time"
)

// OpGetMore is used to query the database for documents in a collection.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-get-more
type GetMoreOp struct {
	Header MsgHeader
	mgo.GetMoreOp
}

func (op *GetMoreOp) OpCode() OpCode {
	return OpCodeGetMore
}

func (op *GetMoreOp) Meta() OpMetadata {
	return OpMetadata{"getmore", op.Collection, "", op.CursorId}
}

func (op *GetMoreOp) String() string {
	return fmt.Sprintf("GetMore ns:%v limit:%v cursorId:%v", op.Collection, op.Limit, op.CursorId)
}
func (op *GetMoreOp) Abbreviated(chars int) string {
	return fmt.Sprintf("%v", op)
}

// getCursorIds implements the cursorsRewriteable interface method. It
// returns an array with one element, containing the cursorId from this getmore.
func (op *GetMoreOp) getCursorIds() ([]int64, error) {
	return []int64{op.CursorId}, nil
}

// setCursorIds implements the cursorsRewriteable interface method. It
// takes an array of cursorIds to replace the current cursor with. If this
// array is longer than 1, it returns an error because getmores can only
// have - and therefore rewrite - one cursor.
func (op *GetMoreOp) setCursorIds(newCursorIds []int64) error {
	var newCursorId int64

	if len(newCursorIds) > 1 {
		return fmt.Errorf("rewriting getmore command cursorIds requires 1 id, received: %d", len(newCursorIds))
	}
	if len(newCursorIds) < 1 {
		newCursorId = 0
	} else {
		newCursorId = newCursorIds[0]
	}
	op.GetMoreOp.CursorId = newCursorId
	return nil
}

func (op *GetMoreOp) FromReader(r io.Reader) error {
	var b [12]byte
	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return err
	}
	name, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Collection = string(name)
	if _, err := io.ReadFull(r, b[:12]); err != nil {
		return err
	}
	op.Limit = getInt32(b[:], 0)
	op.CursorId = getInt64(b[:], 4)
	return nil
}

func (op *GetMoreOp) Execute(session *mgo.Session) (Replyable, error) {
	session.SetSocketTimeout(0)
	before := time.Now()

	_, _, data, resultReply, err := mgo.ExecOpWithReply(session, &op.GetMoreOp)
	after := time.Now()

	mgoReply, ok := resultReply.(*mgo.ReplyOp)
	if !ok {
		panic("reply from execution was not the correct type")
	}

	reply := &ReplyOp{
		ReplyOp: *mgoReply,
		Docs:    make([]bson.Raw, 0, len(data)),
	}

	for _, d := range data {
		dataDoc := bson.Raw{}
		err = bson.Unmarshal(d, &dataDoc)
		if err != nil {
			return nil, err
		}
		reply.Docs = append(reply.Docs, dataDoc)
	}

	reply.Latency = after.Sub(before)
	return reply, nil
}
