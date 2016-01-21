package mongoproto

import (
	"fmt"
	"io"
	"time"

	"github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
)

// ErrNotMsg is returned if a provided buffer is too small to contain a Mongo message
var ErrNotMsg = fmt.Errorf("buffer is too small to be a Mongo message")

type OpMetadata struct {
	// Op represents the actual operation being performed accounting for write commands, so
	// this may be "insert" or "update" even when the wire protocol message was OP_QUERY.
	Op string

	// Namespace against which the operation executes. If not applicable, will be blank.
	Ns string

	// Command name is the name of the command, when Op is "command" (otherwise will be blank.)
	// For example, this might be "getLastError" or "serverStatus".
	Command string
}

// Op is a Mongo operation
type Op interface {
	OpCode() OpCode
	FromReader(io.Reader) error
	Execute(*mgo.Session) (*OpResult, error)
	Equals(Op) bool
	Meta() OpMetadata
}

type OpResult struct {
	ReplyOp *mgo.ReplyOp
	Docs    []bson.D
	Latency time.Duration
}

func (opr *OpResult) String() string {
	if opr == nil {
		return "OpResult NIL"
	}
	return fmt.Sprintf("OpResult latency:%v reply:[flags:%v, cursorid:%v, first:%v ndocs:%v] docs:%s",
		opr.Latency,
		opr.ReplyOp.Flags, opr.ReplyOp.CursorId, opr.ReplyOp.FirstDoc, opr.ReplyOp.ReplyDocs,
		stringifyReplyDocs(opr.Docs))
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
