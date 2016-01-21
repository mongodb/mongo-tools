package mongoplay

import (
	"bytes"
	"fmt"
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/mongoplay/mongoproto"
	"github.com/mongodb/mongo-tools/common/log"
	"sync"
	"time"
)

type ReplyPair struct {
	OpFromFile *mgo.ReplyOp
	OpFromWire *mgo.ReplyOp
}

type ExecutionContext struct {
	IncompleteReplies map[string]ReplyPair
	CompleteReplies   map[string]ReplyPair
	RepliesLock       sync.Mutex
	CursorIDMap       map[int64]int64
	CursorIDMapLock   sync.Mutex
	StatCollector
}

func (context *ExecutionContext) AddFromWire(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.SrcEndpoint + ":" + recordedOp.DstEndpoint + ":" + string(recordedOp.Header.RequestID)
	context.RepliesLock.Lock()
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromWire = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromFile != nil {
		context.completeReply(key)
	}
	context.RepliesLock.Unlock()
}

func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := recordedOp.DstEndpoint + ":" + recordedOp.SrcEndpoint + ":" + string(recordedOp.Header.ResponseTo)
	context.RepliesLock.Lock()
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromFile = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromWire != nil {
		context.completeReply(key)
	}
	context.RepliesLock.Unlock()
}

func (context *ExecutionContext) completeReply(key string) {
	context.CompleteReplies[key] = context.IncompleteReplies[key]
	delete(context.IncompleteReplies, key)
}

func (context *ExecutionContext) fixupOpGetMore(opGM *mongoproto.GetMoreOp) {
	// can race if GetMore's are executed on different connections then the inital query
	context.CursorIDMapLock.Lock()
	cursorId, ok := context.CursorIDMap[opGM.CursorId]
	context.CursorIDMapLock.Unlock()
	if !ok {
		log.Logf(log.Always, "Missing mapped cursor ID for raw cursor ID: %v", opGM.CursorId)
	}
	opGM.CursorId = cursorId
}

func (context *ExecutionContext) handleCompletedReplies() {
	context.RepliesLock.Lock()
	for key, rp := range context.CompleteReplies {
		log.Logf(log.DebugHigh, "Completed reply: %v, %v", rp.OpFromFile, rp.OpFromWire)
		if rp.OpFromFile.CursorId != 0 {
			// can race if GetMore's are executed on different connections then the inital query
			context.CursorIDMapLock.Lock()
			context.CursorIDMap[rp.OpFromFile.CursorId] = rp.OpFromWire.CursorId
			context.CursorIDMapLock.Unlock()
		}
		delete(context.CompleteReplies, key)
	}
	context.RepliesLock.Unlock()
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) error {
	reader := bytes.NewReader(op.OpRaw.Body)

	if op.OpRaw.Header.OpCode == mongoproto.OpCodeReply {
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
			log.Logf(log.Always, "Skipping incomplete op: %v", op.OpRaw.Header.OpCode)
			return nil
		}
		err := opToExec.FromReader(reader)
		if err != nil {
			return err
		}
		if opGM, ok := opToExec.(*mongoproto.GetMoreOp); ok {
			context.fixupOpGetMore(opGM)
		}

		op.PlayedAt = time.Now()
		log.Logf(log.Info, "(Connection %v) [lag: %8s] Executing: %s", op.ConnectionNum, op.PlayedAt.Sub(op.PlayAt), opToExec)
		reply, err := opToExec.Execute(session)

		if err != nil {
			return fmt.Errorf("error executing op: %v", err)
		}

		context.CollectOpInfo(op, opToExec, reply)
		log.Logf(log.DebugLow, "(Connection %v) reply: %s", op.ConnectionNum, reply.String()) //(latency:%v, flags:%v, cursorId:%v, docs:%v) %v", op.ConnectionNum, reply.Latency, reply.ReplyOp.Flags, reply.ReplyOp.CursorId, reply.ReplyOp.ReplyDocs, stringifyReplyDocs(reply.Docs))
		if reply != nil {
			context.AddFromWire(reply.ReplyOp, op)
		}
	}
	context.handleCompletedReplies()

	return nil
}
