package mongotape

import (
	"strconv"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// cursorManager is an interface that defines how to store and retrieve cursorIds
// that are found during live traffic playback.
type cursorManager interface {

	// GetCursor is a function that defines how to retrieve a cursor from the underlying
	// data structure.
	// As arguments it takes the cursorId from the file and the connection number the associated op was played on.
	GetCursor(int64, int64) (int64, bool)

	// SetCursor is a function that defines how to associate a live cursor with one found in the playback file.
	// As arguments it takes the cursorId from the file and the live cursorId received during playback
	SetCursor(int64, int64)

	// MarkFailed is a function to communicate with the manager that an op has failed and no longer wait for
	// the cursorId that it may generate.
	// As an argument, it takes the RecordedOp that failed to execute.
	MarkFailed(*RecordedOp)
}

// cursorCache is an implementation of the cursorManager that uses a ttl cache for storing cursorIds
// during traffic playback. Inserting cursorIds into the cursorCache puts them in with a timeout and
// retrieving a cursor does not block if the cursor is not immediately found in the underlying data structure.
type cursorCache cache.Cache

func newCursorCache() *cursorCache {
	return (*cursorCache)(cache.New(600*time.Second, 60*time.Second))
}

func (c *cursorCache) GetCursor(fileCursorId int64, connectionNum int64) (int64, bool) {
	value, ok := c.Get(strconv.FormatInt(fileCursorId, 10))
	if !ok {
		userInfoLogger.Logf(Always, "Missing mapped cursorId for raw cursorId : %v in GetMoreOp", fileCursorId)
		return 0, false
	}
	return value.(int64), true
}
func (c *cursorCache) MarkFailed(op *RecordedOp) {}

func (c *cursorCache) SetCursor(fileCursorId int64, liveCursorId int64) {
	c.Set(strconv.FormatInt(fileCursorId, 10), liveCursorId, cache.DefaultExpiration)
}

// preprocessCursorManager is an implementation of cursorManager. The preprocessCursorManager holds information
// about the cursorIds seen during preprocessing the file before playback. Setting a cursorId
// from live traffic maps the cursorId found in the preprocessing step to the live cursor. Retrieving
// a cursorId that was entered in the preprocessing step blocks until the live cursorId is received.
type preprocessCursorManager struct {
	cursorInfos map[int64]*preprocessCursorInfo
	opToCursors map[opKey]int64
	sync.RWMutex
}

// preprocessCursorInfo holds information about a cursor that was seen during the preprocessing of the traffic.
type preprocessCursorInfo struct {
	// liveCursorId is the cursorId seen during live playback.
	liveCursorId int64
	// blockChan is a channel that ensures execution of connection will halt until the corresponding cursorId is seen
	// from the live traffic. It is closed in the Set function.
	successChan chan struct{}
	// failChain is a channel that communicates that the op that should have created this cursor failed, so proceeding with
	// executing of the corresponding op should stop
	failChan chan struct{}
	// numUsesLeft is the number of uses the corresponding cursor has in the playbackfile.
	numUsesLeft int
	// replyConn is the connection number that the reply is expected to be on
	replyConn int64
}

type cursorCounter struct {
	opOriginKey opKey
	usesConn    []int64
	replyConn   int64
	replySeen   bool
	usesSeen    int
}

type cursorsSeenMap map[int64]cursorCounter

func (c *cursorsSeenMap) trackSeen(cursorId int64, connectionNum int64) {
	val, ok := (*c)[cursorId]
	if !ok {
		(*c)[cursorId] = cursorCounter{
			usesConn:  []int64{connectionNum},
			replySeen: false,
			usesSeen:  1,
		}
		return
	}
	val.usesSeen++
	val.usesConn = append(val.usesConn, connectionNum)
	(*c)[cursorId] = val
}
func (c *cursorsSeenMap) trackReplied(cursorId int64, op *RecordedOp) {
	key := opKey{
		driverEndpoint: op.DstEndpoint,
		serverEndpoint: op.SrcEndpoint,
		opId:           op.Header.ResponseTo,
	}
	val, ok := (*c)[cursorId]
	if !ok {
		(*c)[cursorId] = cursorCounter{
			replySeen:   true,
			usesSeen:    0,
			replyConn:   op.SeenConnectionNum,
			opOriginKey: key,
		}
		return
	}
	val.replyConn = op.SeenConnectionNum
	val.replySeen = true
	(*c)[cursorId] = val

}

// MarkFailed communicates to any waiting execution sessions that the op associated with certain cursor has failed.
// It closes the failChan for that op so that execution for any sessions waiting on that cursor could continue.
func (p *preprocessCursorManager) MarkFailed(failedOp *RecordedOp) {
	key := opKey{
		driverEndpoint: failedOp.SrcEndpoint,
		serverEndpoint: failedOp.DstEndpoint,
		opId:           failedOp.Header.RequestID,
	}
	if cursor, ok := p.opToCursors[key]; ok {
		if cursorInfo, ok := p.cursorInfos[cursor]; ok {
			close(cursorInfo.failChan)
		}
	}
}

// newPreprocessCursorManager generates a map of cursorIds that were found when preprocessing
// the operations. To perform this, it checks to see if a reply containing a given cursorId
// is seen and a corresponding getmore which uses that cursorId is also seen. It adds these such
// cursorIds to the map and tracks how many uses they have had as well.
func newPreprocessCursorManager(opChan <-chan *RecordedOp) (*preprocessCursorManager, error) {
	userInfoLogger.Logf(Always, "Preprocessing file")

	result := preprocessCursorManager{
		cursorInfos: make(map[int64]*preprocessCursorInfo),
		opToCursors: make(map[opKey]int64),
	}

	cursorsSeen := &cursorsSeenMap{}

	// Loop over all the ops found in the file
	for op := range opChan {

		// If they don't produce a cursor, skip them
		if op.RawOp.Header.OpCode != OpCodeGetMore && op.RawOp.Header.OpCode != OpCodeKillCursors && op.RawOp.Header.OpCode != OpCodeReply {
			continue
		}
		parsedOp, err := op.RawOp.Parse()
		if err != nil {
			return nil, err
		}

		switch castOp := parsedOp.(type) {
		// If the op makes use of a cursor, such as a getmore or a killcursors, track this op and
		// attemp to match it with the reply that contains its cursor

		case *GetMoreOp:
			cursorsSeen.trackSeen(castOp.CursorId, op.SeenConnectionNum)

		case *KillCursorsOp:
			for _, cursorId := range castOp.CursorIds {
				cursorsSeen.trackSeen(cursorId, op.SeenConnectionNum)
			}
			// If the op is a reply it may contain a cursorId. If so, track this op and attempt to
			// pair it with the the op that requires its cursor id.
		case *ReplyOp:

			replyCon := replyContainer{}
			replyCon.ReplyOp = castOp

			cursorId, err := GetCursorId(replyCon)
			if err != nil {
				return nil, err
			}
			if cursorId == 0 {
				continue
			}
			cursorsSeen.trackReplied(cursorId, op)

		}
	}

	for cursorId, counter := range *cursorsSeen {
		if cursorId != 0 && counter.replySeen && counter.usesSeen > 0 {
			result.cursorInfos[cursorId] = &preprocessCursorInfo{
				failChan:    make(chan struct{}),
				successChan: make(chan struct{}),
				numUsesLeft: counter.usesSeen,
				replyConn:   counter.replyConn,
			}
			result.opToCursors[counter.opOriginKey] = cursorId

		}
	}
	userInfoLogger.Logf(Always, "Preprocess complete")
	return &result, nil
}

// GetCursor is an implementation of the cursorManager's GetCursor by the preprocessCursorManager. It takes a cursorId from
// the recorded traffic and returns the corresponding cursorId found during live playback. If the reply that produces the
// corresponding cursorId has not been seen yet during playback, but was in the original recording file, GetCursor will block
// until it receives the cursorId. GetCursor also takes the connection number that the waiting operation will be played on
// so that it will not block if the op is somehow waiting for a reply that has not yet occured and is on the same connection.
// It takes a lock to prevent prevent concurrent accesses to its data structues and so that it can unlock while waiting
// for its cursorId without deadlocking other attempts to access its data.
func (p *preprocessCursorManager) GetCursor(fileCursorId int64, connectionNum int64) (int64, bool) {
	p.RLock()
	cursorInfo, ok := p.cursorInfos[fileCursorId]
	p.RUnlock()
	if !ok {
		return 0, false
	}
	select {
	case <-cursorInfo.successChan:
		//the successChan is closed, so we can continue to the next section to retrieve the cursor
	default:
		if connectionNum == cursorInfo.replyConn {
			// the channels are not closed, and this the same connection we are supposed to be waiting on the reply for
			// therefore, the traffic was read out of order at some point, so we should not block
			return 0, false
		}
		// otherwise, the channel is not closed, but we are not waiting on a cursor form the same connection,
		// so we should proceed to the next case
	}

	select {
	case <-cursorInfo.successChan:
		// the cursor has been set after an op was completed which contained it
		p.Lock()
		cursorInfo.numUsesLeft -= 1
		p.cursorInfos[fileCursorId] = cursorInfo
		cursor := cursorInfo.liveCursorId
		if cursorInfo.numUsesLeft <= 0 {
			delete(p.cursorInfos, fileCursorId)
		}
		p.Unlock()

		return cursor, true
	case <-cursorInfo.failChan:
		//the fail chan was closed, which means no cursor is coming for this op
		return 0, false
	}

}

func (p *preprocessCursorManager) SetCursor(fileCursorId int64, liveCursorId int64) {
	p.Lock()
	cursorInfo, ok := p.cursorInfos[fileCursorId]
	if ok {
		cursorInfo.liveCursorId = liveCursorId
		close(cursorInfo.successChan)
	}
	p.Unlock()
}
