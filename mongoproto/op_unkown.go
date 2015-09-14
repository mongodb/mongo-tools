package mongoproto

import (
	"fmt"
	"io"

	"gopkg.in/mgo.v2"
)

// OpUnknown is not a real mongo Op but represents an unrecognized or corrupted op
type OpUnknown struct {
	Header MsgHeader
	Body   []byte
}

func (op *OpUnknown) String() string {
	return fmt.Sprintf("OpUnkown: %v", op.Header.OpCode)
}
func (op *OpUnknown) OpCode() OpCode {
	return op.Header.OpCode
}

func (op *OpUnknown) FromReader(r io.Reader) error {
	if op.Header.MessageLength < MsgHeaderLen {
		return nil
	}
	op.Body = make([]byte, op.Header.MessageLength-MsgHeaderLen)
	_, err := io.ReadFull(r, op.Body)
	return err
}

func (op *OpUnknown) fromWire(b []byte) {
}

func (op *OpUnknown) toWire() []byte {
	return nil
}

func (op *OpUnknown) Execute(session *mgo.Session) error {
	return nil
}
