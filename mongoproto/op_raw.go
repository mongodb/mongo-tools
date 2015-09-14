package mongoproto

import (
	"fmt"
	"io"
	"io/ioutil"
)

// OpRaw may be exactly the same as OpUnknown.
type OpRaw struct {
	Header MsgHeader
	Body   []byte
}

func (op *OpRaw) String() string {
	return fmt.Sprintf("OpRaw: %v", op.Header.OpCode)
}
func (op *OpRaw) OpCode() OpCode {
	return op.Header.OpCode
}

func (op *OpRaw) FromReader(r io.Reader) error {
	if op.Header.MessageLength < MsgHeaderLen {
		return nil
	}
	if op.Header.MessageLength > MaxMessageSize {
		return fmt.Errorf("wire message size, %v, was greater then the maximum, %v bytes", op.Header.MessageLength, MaxMessageSize)
	}
	op.Body = make([]byte, op.Header.MessageLength-MsgHeaderLen)
	_, err := io.ReadFull(r, op.Body)
	return err
}

func (op *OpRaw) ShortReplyFromReader(r io.Reader) error {
	if op.Header.MessageLength < MsgHeaderLen {
		return nil
	}
	if op.Header.MessageLength > MaxMessageSize {
		return fmt.Errorf("wire message size, %v, was greater then the maximum, %v bytes", op.Header.MessageLength, MaxMessageSize)
	}
	op.Body = make([]byte, 20) // op_replies have an additional 20 bytes of header that we capture
	_, err := io.ReadFull(r, op.Body)
	if err != nil {
		return err
	}
	_, err = io.CopyN(ioutil.Discard, r, int64(op.Header.MessageLength-MsgHeaderLen-20))
	return err
}

func (op *OpRaw) fromWire(b []byte) {
}

func (op *OpRaw) toWire() []byte {
	return nil
}
