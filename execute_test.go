package mongotape

import (
	"strconv"
	"testing"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/mongotape/mongoproto"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
)

func TestCompleteReply(t *testing.T) {
	context := NewExecutionContext(nil)

	log.SetVerbosity(&options.Verbosity{[]bool{true, true, true, true, true}, false})

	// AddFromWire takes a recorded request and a live reply to the re-execution of that reply
	reply1 := &mgo.ReplyOp{
		CursorId: 2500,
	}
	recordedOp1 := &RecordedOp{
		DstEndpoint: "a",
		SrcEndpoint: "b",
		RawOp: mongoproto.RawOp{
			Header: mongoproto.MsgHeader{
				RequestID: 1000,
			},
		},
		Generation: 0,
	}
	context.AddFromWire(reply1, recordedOp1)

	// AddFromFile takes a recorded reply and the contained reply
	reply2 := &mgo.ReplyOp{
		CursorId: 1500,
	}
	recordedOp2 := &RecordedOp{
		DstEndpoint: "b",
		SrcEndpoint: "a",
		RawOp: mongoproto.RawOp{
			Header: mongoproto.MsgHeader{
				ResponseTo: 1000,
			},
		},
		Generation: 0,
	}
	context.AddFromFile(reply2, recordedOp2)
	if len(context.CompleteReplies) != 1 {
		t.Error("replies not completed")
	}
	context.handleCompletedReplies()

	cursorIdLookup, ok := context.CursorIdMap.Get(strconv.FormatInt(1500, 10))
	if !ok {
		t.Error("can't find cursorId in map")
	}
	if cursorIdLookup.(int64) != 2500 {
		t.Errorf("looked up cursorId is wrong: %v, should be 2500", cursorIdLookup)
	}
}
