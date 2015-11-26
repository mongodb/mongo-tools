package mongoplay

import (
	"bytes"
	"fmt"
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/mongoplay/mongoproto"
)

type ReplyPair struct {
	OpFromFile *mgo.ReplyOp
	OpFromWire *mgo.ReplyOp
}

type ExecutionContext struct {
	IncompleteReplies map[string]ReplyPair
	CompleteReplies   map[string]ReplyPair
}

func (context *ExecutionContext) AddFromWire(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.Connection.Src().String() + ":" + recordedOp.Connection.Dst().String() + ":" + string(recordedOp.Header.RequestID)
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromWire = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromFile != nil {
		context.handleCompleted(key)
	}
}

func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.Connection.Dst().String() + ":" + recordedOp.Connection.Src().String() + ":" + string(recordedOp.Header.ResponseTo)
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromFile = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromWire != nil {
		context.handleCompleted(key)
	}
}

func (context *ExecutionContext) handleCompleted(key string) {
	pair := context.IncompleteReplies[key]
	context.CompleteReplies[key] = pair
	delete(context.IncompleteReplies, key)
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) error {
	reader := bytes.NewReader(op.OpRaw.Body)

	if op.OpRaw.Header.OpCode == mongoproto.OpCodeReply {
		fmt.Printf("Correlate OpReply\n")
		opReply := &mongoproto.ReplyOp{Header: op.OpRaw.Header}
		err := opReply.FromReader(reader)
		if err != nil {
			return err
		}
		context.AddFromFile(&opReply.ReplyOp, op)
		return nil
	}

	var opToExec mongoproto.Op
	switch op.OpRaw.Header.OpCode {
	case mongoproto.OpCodeQuery:
		opToExec = &mongoproto.QueryOp{Header: op.OpRaw.Header}
	case mongoproto.OpCodeGetMore:
		opToExec = &mongoproto.GetMoreOp{Header: op.OpRaw.Header}
	case mongoproto.OpCodeInsert:
		opToExec = &mongoproto.InsertOp{Header: op.OpRaw.Header}
	case mongoproto.OpCodeKillCursors:
		opToExec = &mongoproto.KillCursorsOp{Header: op.OpRaw.Header}
	case mongoproto.OpCodeDelete:
		opToExec = &mongoproto.DeleteOp{Header: op.OpRaw.Header}
	case mongoproto.OpCodeUpdate:
		opToExec = &mongoproto.UpdateOp{Header: op.OpRaw.Header}
	default:
		fmt.Printf("Skipping incomplete op: %v\n", op.OpRaw.Header.OpCode)
		return nil
	}
	fmt.Printf("Execute: %v\n", opToExec.OpCode())
	err := opToExec.FromReader(reader)
	if err != nil {
		return err
	}
	reply, err := opToExec.Execute(session)
	if err != nil {
		return err
	}
	if reply != nil {
		context.AddFromWire(reply, op)
	}

	return nil
}
