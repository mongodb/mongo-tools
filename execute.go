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

// AddFromWire adds a from-wire reply to its IncompleteReplies ReplyPair
// and moves that ReplyPair to CompleteReplies if it's complete.
// The index is based on the src/dest of the recordedOp which should be the op
// that this ReplyOp is a reply to.
func (context *ExecutionContext) AddFromWire(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := fmt.Sprintf("%v:%v:%d:%v", recordedOp.SrcEndpoint, recordedOp.DstEndpoint, recordedOp.Header.RequestID, recordedOp.Generation)
	context.RepliesLock.Lock()
	replyPair := context.IncompleteReplies[key]
	replyPair.OpFromWire = reply
	context.IncompleteReplies[key] = replyPair
	if replyPair.OpFromFile != nil {
		context.completeReply(key)
	}
	context.RepliesLock.Unlock()
}

// AddFromWire adds a from-file reply to its IncompleteReplies ReplyPair
// and moves that Replypair to CompleteReplies if it's complete.
// The index is based on the reversed src/dest of the recordedOp which should
// the RecordedOp that this ReplyOp was unmarshaled out of.
func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := fmt.Sprintf("%v:%v:%d:%v", recordedOp.DstEndpoint, recordedOp.SrcEndpoint, recordedOp.Header.ResponseTo, recordedOp.Generation)
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

func (context *ExecutionContext) newOpConnection(url string, connectionNum int64) (SessionWrapper, error) {
	session, err := mgo.Dial(url)
	if err != nil {
		return SessionWrapper{}, err
	}

	ch := make(chan *RecordedOp, 10000)
	done := make(chan bool)

	sessionWrapper := SessionWrapper{ch, done}
	go func() {
		log.Logf(log.Info, "(Connection %v) New connection CREATED.", connectionNum)
		for op := range ch {
			// Populate the op with the connection num it's being played on.
			// This allows it to be used for downstream reporting of stats.
			op.ConnectionNum = connectionNum
			t := time.Now()
			if op.OpRaw.Header.OpCode != mongoproto.OpCodeReply {
				if t.Before(op.PlayAt) {
					time.Sleep(op.PlayAt.Sub(t))
				}
			}
			log.Logf(log.DebugHigh, "(Connection %v) op %v", connectionNum, op.String())
			err = context.Execute(op, session)
			if err != nil {
				log.Logf(log.Always, "context.Execute error: %v", err)
			}
		}
		log.Logf(log.Info, "(Connection %v) Connection ENDED.", connectionNum)
		done <- true
	}()
	return sessionWrapper, nil
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) error {
	reader := bytes.NewReader(op.OpRaw.Body)

	if op.OpRaw.Header.OpCode == mongoproto.OpCodeReply {
		opReply := &mongoproto.ReplyOp{Header: op.OpRaw.Header}
		err := opReply.FromReader(reader)
		if err != nil {
			return fmt.Errorf("opReply.FromReader: %v", err)
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
			return fmt.Errorf("opToExec.FromReader: %v", err)
		}
		if opGM, ok := opToExec.(*mongoproto.GetMoreOp); ok {
			context.fixupOpGetMore(opGM)
		}

		if mongoproto.IsDriverOp(opToExec) {
			return nil
		}

		op.PlayedAt = time.Now()
		log.Logf(log.Info, "(Connection %v) [lag: %8s] Executing: %s", op.ConnectionNum, op.PlayedAt.Sub(op.PlayAt), opToExec)
		result, err := opToExec.Execute(session)

		if err != nil {
			return fmt.Errorf("error executing op: %v", err)
		}

		context.CollectOpInfo(op, opToExec, result)

		if result != nil {
			log.Logf(log.DebugLow, "(Connection %v) reply: %s", op.ConnectionNum, result.String()) //(latency:%v, flags:%v, cursorId:%v, docs:%v) %v", op.ConnectionNum, reply.Latency, reply.ReplyOp.Flags, reply.ReplyOp.CursorId, reply.ReplyOp.ReplyDocs, stringifyReplyDocs(reply.Docs))
			context.AddFromWire(result.ReplyOp, op)
		} else {
			log.Logf(log.DebugHigh, "(Connection %v) nil reply", op.ConnectionNum) //(latency:%v, flags:%v, cursorId:%v, docs:%v) %v", op.ConnectionNum, reply.Latency, reply.ReplyOp.Flags, reply.ReplyOp.CursorId, reply.ReplyOp.ReplyDocs, stringifyReplyDocs(reply.Docs))
		}
	}
	context.handleCompletedReplies()

	return nil
}
