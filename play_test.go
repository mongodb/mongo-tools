package mongoplay

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/10gen/llmgo/bson"
)

func TestRepeatGeneration(t *testing.T) {
	recOp := &RecordedOp{
		Seen: time.Now(),
	}
	bsonBytes, err := bson.Marshal(recOp)
	if err != nil {
		t.Errorf("couldn't marshal %v", err)
	}
	readSeeker := bytes.NewReader(bsonBytes)
	play := PlayCommand{
		Repeat: 2,
	}
	opChan, errChan := play.NewPlayOpChan(readSeeker)
	op1, ok := <-opChan
	if !ok {
		t.Errorf("read of 0-generation op failed")
	}
	if op1.Generation != 0 {
		t.Errorf("generation of 0 generation op is %v", op1.Generation)
	}
	op2, ok := <-opChan
	if !ok {
		t.Errorf("read of 1-generation op failed")
	}
	if op2.Generation != 1 {
		t.Errorf("generation of 1 generation op is %v", op2.Generation)
	}
	_, ok = <-opChan
	if ok {
		t.Errorf("Successfully read past end of op chan")
	}
	err = <-errChan
	if err != io.EOF {
		t.Errorf("should have eof at end, but got %v", err)
	}
}
