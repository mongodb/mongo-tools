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
	IncompleteReplies     map[string]ReplyPair
	CompleteReplies       map[string]ReplyPair
	RepliesLock           sync.Mutex
	CursorIDMap           map[int64]int64
	CursorIDMapLock       sync.Mutex
	SessionChansWaitGroup sync.WaitGroup
	StatCollector
}

func NewExecutionContext(statColl StatCollector) *ExecutionContext {
	context := ExecutionContext{
		IncompleteReplies: map[string]ReplyPair{},
		CompleteReplies:   map[string]ReplyPair{},
		CursorIDMap:       map[int64]int64{},
		StatCollector:     statColl,
	}
	return &context
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

// AddFromFile adds a from-file reply to its IncompleteReplies ReplyPair
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

func (context *ExecutionContext) newExecutionSession(url string, start time.Time, connectionNum int64) chan<- *RecordedOp {

	ch := make(chan *RecordedOp, 10000)

	context.SessionChansWaitGroup.Add(1)
	go func() {
		now := time.Now()
		var connected bool
		time.Sleep(start.Add(-5 * time.Second).Sub(now)) // Sleep until five seconds before the start time
		session, err := mgo.Dial(url)
		if err == nil {
			log.Logf(log.Info, "(Connection %v) New connection CREATED.", connectionNum)
			connected = true
		} else {
			log.Logf(log.Info, "(Connection %v) New Connection FAILED: %v", connectionNum, err)
		}
		for op := range ch {
			if connected {
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
				session.SetSocketTimeout(0)
				err = context.Execute(op, session)
				if err != nil {
					log.Logf(log.Always, "context.Execute error: %v", err)
				}
			} else {
				log.Logf(log.DebugHigh, "(Connection %v) SKIPPING op on non-connected session %v", connectionNum, op.String())
			}
		}
		log.Logf(log.Info, "(Connection %v) Connection ENDED.", connectionNum)
		context.SessionChansWaitGroup.Done()
	}()
	return ch
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) error {
	reader := bytes.NewReader(op.OpRaw.Body[mongoproto.MsgHeaderLen:])

	if op.OpRaw.Header.OpCode == mongoproto.OpCodeReply {
		opReply := &mongoproto.ReplyOp{Header: op.OpRaw.Header}
		err := opReply.FromReader(reader)
		if err != nil {
			return fmt.Errorf("opReply.FromReader: %v", err)
		}
		cursorId, err := mongoproto.GetCursorId(&(opReply.ReplyOp), opReply.Docs)
		if err != nil {
			log.Logf(log.Always, "Warning: error when trying to find cursor ID in reply: %v", err)
		}
		log.Logf(log.DebugHigh, "Adding reply from file: %v, %v %v docs, cursorId %v", op.OpRaw.Header, opReply.ReplyDocs, len(opReply.Docs), cursorId)
		if cursorId != 0 {
			opReply.ReplyOp.CursorId = cursorId
		}
		// release docs data to garbage collection
		opReply.Docs = nil
		context.AddFromFile(&opReply.ReplyOp, op)
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

		if result != nil && result.ReplyOp != nil {
			// Check verbosity level before entering this block to avoid
			// the performance penalty of evaluating reply.String() unless necessary.
			if log.IsInVerbosity(log.DebugHigh) {
				log.Logf(log.DebugHigh, "(Connection %v) reply: %s", op.ConnectionNum, result)
			}
			cursorId, err := mongoproto.GetCursorId(result.ReplyOp, result.Docs)
			log.Logf(log.Always, "Warning: error when trying to find cursor ID in reply: %v", err)
			if cursorId != 0 {
				result.ReplyOp.CursorId = cursorId
			}
			context.AddFromWire(result.ReplyOp, op)
		} else {
			log.Logf(log.DebugHigh, "(Connection %v) nil reply", op.ConnectionNum)
		}
	}
	context.handleCompletedReplies()

	return nil
}
