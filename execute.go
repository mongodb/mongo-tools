package mongoplay

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/mongoplay/mongoproto"
	"github.com/mongodb/mongo-tools/common/log"

	"github.com/patrickmn/go-cache"
)

type ReplyPair struct {
	ops [2]*mgo.ReplyOp
}

const (
	ReplyFromWire = 0
	ReplyFromFile = 1
)

type ExecutionContext struct {
	// IncompleteReplies holds half complete ReplyPairs, which contains either a
	// live reply or a recorded reply when one arrives before the other.
	IncompleteReplies *cache.Cache

	// CompleteReplies contains ReplyPairs that have been competed by the arrival
	// of the missing half of.
	CompleteReplies map[string]*ReplyPair

	// CursorIdMap contains the mapping between recorded cursorIds and live cursorIds
	CursorIdMap *cache.Cache

	// lock synchronizes access to all of the caches and maps in the ExecutionContext
	lock sync.Mutex

	SessionChansWaitGroup sync.WaitGroup
	StatCollector
}

func NewExecutionContext(statRec StatRecorder) *ExecutionContext {
	statColl := StatCollector{
		StatRecorder: statRec,
	}
	return &ExecutionContext{
		IncompleteReplies: cache.New(60*time.Second, 60*time.Second),
		CompleteReplies:   map[string]*ReplyPair{},
		CursorIdMap:       cache.New(600*time.Second, 60*time.Second),
		StatCollector:     statColl,
	}
}

// AddFromWire adds a from-wire reply to its IncompleteReplies ReplyPair
// and moves that ReplyPair to CompleteReplies if it's complete.
// The index is based on the src/dest of the recordedOp which should be the op
// that this ReplyOp is a reply to.
func (context *ExecutionContext) AddFromWire(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := fmt.Sprintf("%v:%v:%d:%v", recordedOp.SrcEndpoint, recordedOp.DstEndpoint, recordedOp.Header.RequestID, recordedOp.Generation)
	context.completeReply(key, reply, ReplyFromWire)
}

// AddFromFile adds a from-file reply to its IncompleteReplies ReplyPair
// and moves that ReplyPair to CompleteReplies if it's complete.
// The index is based on the reversed src/dest of the recordedOp which should
// the RecordedOp that this ReplyOp was unmarshaled out of.
func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := fmt.Sprintf("%v:%v:%d:%v", recordedOp.DstEndpoint, recordedOp.SrcEndpoint, recordedOp.Header.ResponseTo, recordedOp.Generation)
	context.completeReply(key, reply, ReplyFromFile)
}

func (context *ExecutionContext) completeReply(key string, reply *mgo.ReplyOp, opSource int) {
	context.lock.Lock()
	if cacheValue, ok := context.IncompleteReplies.Get(key); !ok {
		rp := &ReplyPair{}
		rp.ops[opSource] = reply
		context.IncompleteReplies.Set(key, rp, cache.DefaultExpiration)
	} else {
		rp := cacheValue.(*ReplyPair)
		rp.ops[opSource] = reply
		if rp.ops[1-opSource] != nil {
			context.CompleteReplies[key] = rp
			context.IncompleteReplies.Delete(key)
		}
	}
	context.lock.Unlock()
}

func (context *ExecutionContext) fixupOpGetMore(opGM *mongoproto.GetMoreOp) {
	context.lock.Lock()
	value, ok := context.CursorIdMap.Get(strconv.FormatInt(opGM.CursorId, 10))
	context.lock.Unlock()
	if !ok {
		log.Logf(log.Always, "Missing mapped cursor ID for raw cursor ID: %v", opGM.CursorId)
	} else {
		opGM.CursorId = value.(int64)
	}
}

func (context *ExecutionContext) handleCompletedReplies() {
	context.lock.Lock()
	for key, rp := range context.CompleteReplies {
		log.Logf(log.DebugHigh, "Completed reply: %v, %v", rp.ops[ReplyFromFile], rp.ops[ReplyFromWire])
		if rp.ops[ReplyFromFile].CursorId != 0 {
			context.CursorIdMap.Set(strconv.FormatInt(rp.ops[ReplyFromFile].CursorId, 10), rp.ops[ReplyFromWire].CursorId, cache.DefaultExpiration)
		}
		delete(context.CompleteReplies, key)
	}
	context.lock.Unlock()
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
		for recordedOp := range ch {
			var parsedOp mongoproto.Op
			var reply *mongoproto.ReplyOp
			var err error
			msg := ""
			if connected {
				// Populate the op with the connection num it's being played on.
				// This allows it to be used for downstream reporting of stats.
				recordedOp.ConnectionNum = connectionNum
				t := time.Now()
				if recordedOp.OpRaw.Header.OpCode != mongoproto.OpCodeReply {
					if t.Before(recordedOp.PlayAt) {
						time.Sleep(recordedOp.PlayAt.Sub(t))
					}
				}
				log.Logf(log.DebugHigh, "(Connection %v) op %v", connectionNum, recordedOp.String())
				session.SetSocketTimeout(0)
				parsedOp, reply, err = context.Execute(recordedOp, session)
				if err != nil {
					log.Logf(log.Always, "context.Execute error: %v", err)
				}
			} else {
				parsedOp, err = recordedOp.Parse()
				if err != nil {
					log.Logf(log.Always, "Execution Session error: %v", err)
				}

				msg = fmt.Sprintf("Skipped on non-connected session (Connection %v)", connectionNum)
			}
			if shouldCollectOp(parsedOp) {
				context.Collect(recordedOp, parsedOp, reply, msg)
			}
		}
		log.Logf(log.Info, "(Connection %v) Connection ENDED.", connectionNum)
		context.SessionChansWaitGroup.Done()
	}()
	return ch
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) (mongoproto.Op, *mongoproto.ReplyOp, error) {
	opToExec, err := op.OpRaw.Parse()
	var replyOp *mongoproto.ReplyOp

	if err != nil {
		return nil, nil, fmt.Errorf("ParseOpRawError: %v", err)
	}
	if opToExec == nil {
		log.Logf(log.Always, "Skipping incomplete op: %v", op.OpRaw.Header.OpCode)
		return nil, nil, nil
	}
	if recordedReply, ok := opToExec.(*mongoproto.ReplyOp); ok {
		context.AddFromFile(&recordedReply.ReplyOp, op)
	} else {

		if opGM, ok := opToExec.(*mongoproto.GetMoreOp); ok {
			context.fixupOpGetMore(opGM)
		}

		if mongoproto.IsDriverOp(opToExec) {
			return opToExec, replyOp, nil
		}

		op.PlayedAt = time.Now()
		log.Logf(log.Info, "(Connection %v) [lag: %8s] Executing: %s", op.ConnectionNum, op.PlayedAt.Sub(op.PlayAt), opToExec)
		replyOp, err = opToExec.Execute(session)

		if err != nil {
			return opToExec, replyOp, fmt.Errorf("error executing op: %v", err)
		}

		if replyOp != nil && &replyOp.ReplyOp != nil {
			// Check verbosity level before entering this block to avoid
			// the performance penalty of evaluating reply.String() unless necessary.
			if log.IsInVerbosity(log.DebugHigh) {
				log.Logf(log.DebugHigh, "(Connection %v) reply: %s", op.ConnectionNum, replyOp)
			}
			cursorId, err := mongoproto.GetCursorId(&replyOp.ReplyOp, replyOp.Docs)
			if err != nil {
				log.Logf(log.Always, "Warning: error when trying to find cursor ID in reply: %v", err)
			}
			if cursorId != 0 {
				replyOp.ReplyOp.CursorId = cursorId
			}
			context.AddFromWire(&replyOp.ReplyOp, op)
		} else {
			log.Logf(log.DebugHigh, "(Connection %v) nil reply", op.ConnectionNum)
		}
	}
	context.handleCompletedReplies()

	return opToExec, replyOp, nil
}
