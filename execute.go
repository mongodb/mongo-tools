package mongotape

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	mgo "github.com/10gen/llmgo"
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
	sync.RWMutex

	SessionChansWaitGroup sync.WaitGroup

	*StatCollector
}

func NewExecutionContext(statColl *StatCollector) *ExecutionContext {
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
	toolDebugLogger.Logf(DebugHigh, "Adding live reply with key %v", key)
	context.completeReply(key, reply, ReplyFromWire)
}

// AddFromFile adds a from-file reply to its IncompleteReplies ReplyPair
// and moves that ReplyPair to CompleteReplies if it's complete.
// The index is based on the reversed src/dest of the recordedOp which should
// the RecordedOp that this ReplyOp was unmarshaled out of.
func (context *ExecutionContext) AddFromFile(reply *mgo.ReplyOp, recordedOp *RecordedOp) {
	key := fmt.Sprintf("%v:%v:%d:%v", recordedOp.DstEndpoint, recordedOp.SrcEndpoint, recordedOp.Header.ResponseTo, recordedOp.Generation)
	toolDebugLogger.Logf(DebugHigh, "Adding recorded reply with key %v", key)
	context.completeReply(key, reply, ReplyFromFile)
}

func (context *ExecutionContext) completeReply(key string, reply *mgo.ReplyOp, opSource int) {
	context.Lock()
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
	context.Unlock()
}

func (context *ExecutionContext) fixupGetMoreOp(getmoreOp *GetMoreOp) {
	userInfoLogger.Logf(DebugLow, "Rewriting getmore cursor with ID: %v", getmoreOp.CursorId)
	context.RLock()
	value, ok := context.CursorIdMap.Get(strconv.FormatInt(getmoreOp.CursorId, 10))
	context.RUnlock()
	if !ok {
		userInfoLogger.Logf(Always, "Missing mapped cursorId for raw cursorId : %v in GetMoreOp", getmoreOp.CursorId)
	} else {
		getmoreOp.CursorId = value.(int64)
	}
}

func (context *ExecutionContext) fixupKillCursorsOp(killcursorsOp *KillCursorsOp) {

	index := 0
	for _, cursorId := range killcursorsOp.CursorIds {
		userInfoLogger.Logf(DebugLow, "Rewriting killcursors cursorId : %v", cursorId)
		context.RLock()
		value, ok := context.CursorIdMap.Get(strconv.FormatInt(cursorId, 10))
		context.RUnlock()
		if ok {
			killcursorsOp.CursorIds[index] = value.(int64)
			index++
		} else {
			userInfoLogger.Logf(Always, "Missing mapped cursorId for raw cursorId : %v in KillCursorsOp", cursorId)
		}
	}
	killcursorsOp.CursorIds = killcursorsOp.CursorIds[0:index]
}

func (context *ExecutionContext) handleCompletedReplies() {
	context.Lock()
	for key, rp := range context.CompleteReplies {
		userInfoLogger.Logf(DebugHigh, "Completed reply: %#v, %#v", rp.ops[ReplyFromFile], rp.ops[ReplyFromWire])
		if rp.ops[ReplyFromFile].CursorId != 0 {
			context.CursorIdMap.Set(strconv.FormatInt(rp.ops[ReplyFromFile].CursorId, 10), rp.ops[ReplyFromWire].CursorId, cache.DefaultExpiration)
		}
		delete(context.CompleteReplies, key)
	}
	context.Unlock()
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
			userInfoLogger.Logf(Info, "(Connection %v) New connection CREATED.", connectionNum)
			connected = true
		} else {
			userInfoLogger.Logf(Info, "(Connection %v) New Connection FAILED: %v", connectionNum, err)
		}
		for recordedOp := range ch {
			var parsedOp Op
			var replyContainer replyContainer
			var err error
			msg := ""
			if connected {
				// Populate the op with the connection num it's being played on.
				// This allows it to be used for downstream reporting of stats.
				recordedOp.ConnectionNum = connectionNum
				t := time.Now()
				if recordedOp.RawOp.Header.OpCode != OpCodeReply {
					if t.Before(recordedOp.PlayAt.Time) {
						time.Sleep(recordedOp.PlayAt.Sub(t))
					}
				}
				userInfoLogger.Logf(DebugHigh, "(Connection %v) op %v", connectionNum, recordedOp.String())
				session.SetSocketTimeout(0)
				parsedOp, replyContainer, err = context.Execute(recordedOp, session)
				if err != nil {
					toolDebugLogger.Logf(Always, "context.Execute error: %v", err)
				}
			} else {
				parsedOp, err = recordedOp.Parse()
				if err != nil {
					toolDebugLogger.Logf(Always, "Execution Session error: %v", err)
				}

				msg = fmt.Sprintf("Skipped on non-connected session (Connection %v)", connectionNum)
				toolDebugLogger.Log(Always, msg)
			}
			if shouldCollectOp(parsedOp) {
				context.Collect(recordedOp, parsedOp, replyContainer, msg)
			}
		}
		userInfoLogger.Logf(Info, "(Connection %v) Connection ENDED.", connectionNum)
		context.SessionChansWaitGroup.Done()
	}()
	return ch
}

func (context *ExecutionContext) Execute(op *RecordedOp, session *mgo.Session) (Op, replyContainer, error) {
	opToExec, err := op.RawOp.Parse()
	var replyContainer replyContainer

	if err != nil {
		return nil, replyContainer, fmt.Errorf("ParseOpRawError: %v", err)
	}
	if opToExec == nil {
		toolDebugLogger.Logf(Always, "Skipping incomplete op: %v", op.RawOp.Header.OpCode)
		return nil, replyContainer, nil
	}
	if recordedReply, ok := opToExec.(*ReplyOp); ok {
		context.AddFromFile(&recordedReply.ReplyOp, op)
	} else if _, ok := opToExec.(*CommandReplyOp); ok {
		// XXX handle the CommandReplyOp and pair it with it's other one from the wire
	} else {
		if IsDriverOp(opToExec) {
			return opToExec, replyContainer, nil
		}

		switch t := opToExec.(type) {
		case *GetMoreOp:
			context.fixupGetMoreOp(t)
		case *KillCursorsOp:
			context.fixupKillCursorsOp(t)
		}

		op.PlayedAt = &PreciseTime{time.Now()}
		replyContainer, err = opToExec.Execute(session)

		if err != nil {
			return opToExec, replyContainer, fmt.Errorf("error executing op: %v", err)
		}
		if replyContainer.ReplyOp != nil {
			cursorId, err := GetCursorId(replyContainer)
			if err != nil {
				toolDebugLogger.Logf(Always, "Warning: error when trying to find cursor ID in reply: %v", err)
			}
			if cursorId != 0 {
				replyContainer.ReplyOp.CursorId = cursorId
			}
			context.AddFromWire(&replyContainer.ReplyOp.ReplyOp, op)
		}

	}
	context.handleCompletedReplies()

	return opToExec, replyContainer, nil
}
