package mongoproto

import (
	"fmt"
	"io"
	"time"

	"github.com/10gen/llmgo"
	"github.com/10gen/mongoplay/tcpreader"
)

const (
	MaxMessageSize = 48 << 20 // 48 MB
)

// ErrNotMsg is returned if a provided buffer is too small to contain a Mongo message
var ErrNotMsg = fmt.Errorf("buffer is too small to be a Mongo message")

// Op is a Mongo operation
type Op interface {
	OpCode() OpCode
	FromReader(io.Reader) error
	Execute(*mgo.Session) (*mgo.ReplyOp, error)
	Equals(Op) bool
}

// ErrUnknownOpcode is an error that represents an unrecognized opcode.
type ErrUnknownOpcode int

func (e ErrUnknownOpcode) Error() string {
	return fmt.Sprintf("Unknown opcode %d", e)
}

// OpFromReader reads an Op from an io.Reader
func OpFromReader(r io.Reader) (Op, error) {
	msg, err := ReadHeader(r)
	if err != nil {
		return nil, err
	}
	m := *msg

	var result Op
	switch m.OpCode {
	case OpCodeQuery:
		result = &QueryOp{Header: m}
	case OpCodeReply:
		result = &ReplyOp{Header: m}
	case OpCodeGetMore:
		result = &GetMoreOp{Header: m}
	case OpCodeInsert:
		result = &InsertOp{Header: m}
	case OpCodeDelete:
		result = &DeleteOp{Header: m}
	case OpCodeKillCursors:
		result = &KillCursorsOp{Header: m}
	case OpCodeUpdate:
		result = &UpdateOp{Header: m}
	default:
		result = &OpUnknown{Header: m}
	}
	err = result.FromReader(r)
	return result, err
}

// OpRawFromReader reads an op without decoding it.
func OpRawFromReader(r io.Reader) (*OpRaw, *MsgHeader, time.Time, error) {
	var seen time.Time
	msg, err := ReadHeader(r)
	if err != nil {
		return nil, nil, seen, err
	}
	if readerStream, ok := (r).(*tcpreader.ReaderStream); ok {
		seen = readerStream.Seen()
	}
	result := &OpRaw{Header: *msg}
	if msg.OpCode == 1 {
		err = result.ShortReplyFromReader(r)
	} else {
		err = result.FromReader(r)
	}
	if err != nil {
		return result, msg, seen, err
	}
	return result, msg, seen, nil
}
