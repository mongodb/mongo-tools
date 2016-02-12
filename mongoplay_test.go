package mongoplay

import (
	"testing"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
	"reflect"
)

type testDoc struct {
	Name           string `bson:"name"`
	DocumentNumber int    `bson:"docNum"`
	Success        bool   `bson:"success"`
}

func TestOpGetMore(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.GetMoreOp{}
	op.Collection = "mongoplay_test.test"
	op.CursorId = 12345
	op.Limit = -1

	result, err := generator.fetchRecordedOpsFromConn(&op.GetMoreOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}
	getMoreOp := receivedOp.(*mongoproto.GetMoreOp)

	switch {
	case getMoreOp.Collection != "mongoplay_test.test":
		t.Fail()
	case getMoreOp.CursorId != 12345:
		t.Fail()
	case getMoreOp.Limit != -1:
		t.Fail()
	}
}

func TestOpDelete(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.DeleteOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 7
	selector := bson.D{{"test", 1}}
	op.Selector = selector

	result, err := generator.fetchRecordedOpsFromConn(&op.DeleteOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}
	deleteOp := receivedOp.(*mongoproto.DeleteOp)

	switch {
	case deleteOp.Collection != "mongoplay_test.test":
		t.Fail()
	case deleteOp.Flags != 7:
		t.Fail()
	case !reflect.DeepEqual(selector, op.Selector):
		t.Fail()
	}
}

func TestInsertOp(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.InsertOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 7

	documents := []interface{}(nil)
	for i := 0; i < 10; i++ {
		insertDoc := &testDoc{
			DocumentNumber: i,
			Success:        true,
		}
		documents = append(documents, insertDoc)
	}
	op.Documents = documents

	result, err := generator.fetchRecordedOpsFromConn(&op.InsertOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}
	insertOp := receivedOp.(*mongoproto.InsertOp)

	switch {
	case insertOp.Collection != "mongoplay_test.test":
		t.Fail()
	case insertOp.Flags != 7:
		t.Fail()
	}

	for i, doc := range insertOp.Documents {
		marshaled, _ := bson.Marshal(documents[i])
		unmarshaled := &bson.D{}
		bson.Unmarshal(marshaled, unmarshaled)
		if !reflect.DeepEqual(unmarshaled, doc) {
			t.Fatalf("Documents not equal %v -- %v\n", unmarshaled, doc)
		}
	}
}

func TestKillCursorsOp(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.KillCursorsOp{}
	op.CursorIds = []int64{123, 456, 789, 55}

	result, err := generator.fetchRecordedOpsFromConn(&op.KillCursorsOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}
	killCursorsOp := receivedOp.(*mongoproto.KillCursorsOp)

	if !reflect.DeepEqual(killCursorsOp.CursorIds, op.CursorIds) {
		t.Fatalf("CursorId Arrays not equal %v -- %v\n", killCursorsOp.CursorIds, op.CursorIds)
	}
}

func TestQueryOp(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.QueryOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 0
	op.HasOptions = true
	op.Limit = -1
	op.Skip = 0
	selector := bson.D{{"test", 1}}
	op.Selector = selector
	options := mgo.QueryWrapper{}
	options.Explain = false
	options.OrderBy = &bson.D{{"_id", 1}}
	op.Options = options

	result, err := generator.fetchRecordedOpsFromConn(&op.QueryOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}
	queryOp := receivedOp.(*mongoproto.QueryOp)

	switch {
	case queryOp.Collection != op.Collection:
		t.Fatalf("Collections not equal: %v -- %v\n", queryOp.Collection, op.Collection)
	case !reflect.DeepEqual(&selector, queryOp.Selector):
		t.Fatalf("Selectors not equal: %v -- %v\n", queryOp.Selector, &selector)
	case queryOp.Flags != op.Flags:
		t.Fatalf("Flags not equal: %d -- %d\n", queryOp.Flags, op.Flags)
	case queryOp.Skip != op.Skip:
		t.Fatalf("Skips not equal: %d -- %d\n", queryOp.Skip, op.Skip)
	case queryOp.Limit != op.Limit:
		t.Fatalf("Limits not equal: %d -- %d\n", queryOp.Limit, op.Limit)
	}
	//currently we do not test the Options functionality of mgo
}

func TestOpUpdate(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.UpdateOp{}
	selector := bson.D{{"test", 1}}
	op.Selector = selector
	update := bson.D{{"$set", bson.D{{"updated", true}}}}
	op.Update = update
	op.Collection = "mongoplay_test.test"
	op.Flags = 12345

	result, err := generator.fetchRecordedOpsFromConn(&op.UpdateOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := mongoproto.ParseOpRaw(&result.op.OpRaw)
	if err != nil {
		t.Error(err)
	}

	updateOp := receivedOp.(*mongoproto.UpdateOp)
	switch {
	case updateOp.Collection != op.Collection:
		t.Fatalf("Collections not equal: %v -- %v\n", updateOp.Collection, op.Collection)
	case !reflect.DeepEqual(updateOp.Selector, &selector):
		t.Fatalf("Selectors not equal: %v -- %v\n", updateOp.Selector, &selector)
	case !reflect.DeepEqual(updateOp.Update, &update):
		t.Fatalf("Updates not equal: %v -- %v\n", updateOp.Update, &update)
	case updateOp.Flags != op.Flags:
		t.Fatalf("Flags not equal: %d -- %d\n", updateOp.Flags, op.Flags)
	}
}
