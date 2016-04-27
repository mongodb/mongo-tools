package mongotape

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
)

// RawOp may be exactly the same as OpUnknown.
type RawOp struct {
	Header MsgHeader
	Body   []byte
}

func (op *RawOp) String() string {
	return fmt.Sprintf("RawOp: %v", op.Header.OpCode)
}

func (op *RawOp) Abbreviated(chars int) string {
	return fmt.Sprintf("%v", op)
}

func (op *RawOp) OpCode() OpCode {
	return op.Header.OpCode
}

func (op *RawOp) FromReader(r io.Reader) error {
	if op.Header.MessageLength < MsgHeaderLen {
		return nil
	}
	if op.Header.MessageLength > MaxMessageSize {
		return fmt.Errorf("wire message size, %v, was greater then the maximum, %v bytes", op.Header.MessageLength, MaxMessageSize)
	}
	tempBody := make([]byte, op.Header.MessageLength-MsgHeaderLen)
	_, err := io.ReadFull(r, tempBody)

	if op.Body != nil {
		op.Body = append(op.Body, tempBody...)
	} else {
		op.Body = tempBody
	}
	return err
}

func (op *RawOp) ShortReplyFromReader(r io.Reader) error {
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
func (rawOp *RawOp) Parse() (Op, error) {
	var parsedOp Op
	switch rawOp.Header.OpCode {
	case OpCodeQuery:
		parsedOp = &QueryOp{Header: rawOp.Header}
	case OpCodeReply:
		parsedOp = &ReplyOp{Header: rawOp.Header}
	case OpCodeGetMore:
		parsedOp = &GetMoreOp{Header: rawOp.Header}
	case OpCodeInsert:
		parsedOp = &InsertOp{Header: rawOp.Header}
	case OpCodeKillCursors:
		parsedOp = &KillCursorsOp{Header: rawOp.Header}
	case OpCodeDelete:
		parsedOp = &DeleteOp{Header: rawOp.Header}
	case OpCodeUpdate:
		parsedOp = &UpdateOp{Header: rawOp.Header}
	default:
		return nil, nil
	}
	reader := bytes.NewReader(rawOp.Body[MsgHeaderLen:])
	err := parsedOp.FromReader(reader)
	if err != nil {
		return nil, err
	}
	return parsedOp, nil

}
