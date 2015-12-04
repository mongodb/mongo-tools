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
	// the management (put/fetch) of the CursorIDMap should be moved in to its own goroutine such that
	// we can elliminate races
	CursorIDMap map[int64]int64
}

func (context *ExecutionContext) AddFromWire(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.Connection.Src().String() + ":" + recordedOp.Connection.Dst().String() + ":" + string(recordedOp.Header.RequestID)
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromWire = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromFile != nil {
		context.completeReply(key)
	}
}

func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.Connection.Dst().String() + ":" + recordedOp.Connection.Src().String() + ":" + string(recordedOp.Header.ResponseTo)
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromFile = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromWire != nil {
		context.completeReply(key)
	}
}

func (context *ExecutionContext) completeReply(key string) {
	context.CompleteReplies[key] = context.IncompleteReplies[key]
	delete(context.IncompleteReplies, key)
}

func (context *ExecutionContext) fixupOpGetMore(opGM *mongoproto.GetMoreOp) {
	// can race if GetMore's are executed on different connections then the inital query
	cursorId, ok := context.CursorIDMap[opGM.CursorId]
	if !ok {
		fmt.Printf("MISSING CURSORID FOR CURSORID %v\n", opGM.CursorId)
	}
	opGM.CursorId = cursorId
}

func (context *ExecutionContext) handleCompletedReplies() {
	for key, rp := range context.CompleteReplies {
		fmt.Printf("completed reply: %#v %#v\n", rp.OpFromFile, rp.OpFromWire)
		if rp.OpFromFile.CursorId != 0 {
			// can race if GetMore's are executed on different connections then the inital query
			context.CursorIDMap[rp.OpFromFile.CursorId] = rp.OpFromWire.CursorId
		}
		delete(context.CompleteReplies, key)
	}
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
	} else {

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
		if opGM, ok := opToExec.(*mongoproto.GetMoreOp); ok {
			context.fixupOpGetMore(opGM)
		}
		reply, err := opToExec.Execute(session)
		if err != nil {
			return err
		}
		if reply != nil {
			context.AddFromWire(reply, op)
		}
	}
	context.handleCompletedReplies()

	return nil
}
