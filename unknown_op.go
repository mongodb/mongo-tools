package mongotape

import (
	"fmt"
	"io"

	mgo "github.com/10gen/llmgo"
)

// UnknownOp is not a real mongo Op but represents an unrecognized or corrupted op
type UnknownOp struct {
	Header MsgHeader
	Body   []byte
}

func (op *UnknownOp) Meta() OpMetadata {
	return OpMetadata{"", "", "", nil}
}

func (op *UnknownOp) String() string {
	return fmt.Sprintf("OpUnkown: %v", op.Header.OpCode)
}
func (op *UnknownOp) Abbreviated(chars int) string {
	return fmt.Sprintf("%v", op)
}
func (op *UnknownOp) OpCode() OpCode {
	return op.Header.OpCode
}

func (op *UnknownOp) FromReader(r io.Reader) error {
	if op.Header.MessageLength < MsgHeaderLen {
		return nil
	}
	op.Body = make([]byte, op.Header.MessageLength-MsgHeaderLen)
	_, err := io.ReadFull(r, op.Body)
	return err
}

func (op *UnknownOp) Execute(session *mgo.Session) (*ReplyOp, error) {
	return nil, nil
}
func (unknownOp *UnknownOp) Equals(otherOp Op) bool {
	return true
}
