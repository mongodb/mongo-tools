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

	t.Logf("Generated Getmore: %#v\n", op.GetMoreOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.GetMoreOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}
	getMoreOp := receivedOp.(*mongoproto.GetMoreOp)

	t.Log("Comparing parsed Getmore to original Getmore")
	switch {
	case getMoreOp.Collection != "mongoplay_test.test":
		t.Errorf("Collection not matched. Saw %v -- Expected %v\n", getMoreOp.Collection, "mongoplay_test.test")
	case getMoreOp.CursorId != 12345:
		t.Errorf("CursorId not matched. Saw %v -- Expected %v\n", getMoreOp.CursorId, 12345)
	case getMoreOp.Limit != -1:
		t.Errorf("Limit not matched. Saw %v -- Expected %v\n", getMoreOp.Limit, -1)
	}
}

func TestOpDelete(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.DeleteOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 7
	selector := bson.D{{"test", 1}}
	op.Selector = selector

	t.Logf("Generated Delete: %#v\n", op.DeleteOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.DeleteOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}
	deleteOp := receivedOp.(*mongoproto.DeleteOp)

	t.Log("Comparing parsed Delete to original Delete")
	switch {
	case deleteOp.Collection != "mongoplay_test.test":
		t.Errorf("Collection not matched. Saw %v -- Expected %v\n", deleteOp.Collection, "mongoplay_test.test")
	case deleteOp.Flags != 7:
		t.Errorf("Flags not matched. Saw %v -- Expected %v\n", deleteOp.Flags, 7)
	case !reflect.DeepEqual(deleteOp.Selector, &selector):
		t.Errorf("Selector not matched. Saw %v -- Expected %v\n", deleteOp.Selector, &selector)
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
	t.Logf("Generated Insert: %#v\n", op.InsertOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.InsertOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}
	insertOp := receivedOp.(*mongoproto.InsertOp)

	t.Log("Comparing parsed Insert to original Insert")
	switch {
	case insertOp.Collection != "mongoplay_test.test":
		t.Errorf("Collection not matched. Saw %v -- Expected %v\n", insertOp.Collection, "mongoplay_test.test")
	case insertOp.Flags != 7:
		t.Errorf("Flags not matched. Saw %v -- Expected %v\n", insertOp.Flags, 7)
	}

	for i, doc := range insertOp.Documents {
		marshaled, _ := bson.Marshal(documents[i])
		unmarshaled := &bson.D{}
		bson.Unmarshal(marshaled, unmarshaled)
		if !reflect.DeepEqual(unmarshaled, doc) {
			t.Errorf("Document not matched. Saw %v -- Expected %v\n", unmarshaled, doc)
		}
	}
}

func TestKillCursorsOp(t *testing.T) {
	generator := newRecordedOpGenerator()

	op := mongoproto.KillCursorsOp{}
	op.CursorIds = []int64{123, 456, 789, 55}

	t.Logf("Generated KillCursors: %#v\n", op.KillCursorsOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.KillCursorsOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}
	killCursorsOp := receivedOp.(*mongoproto.KillCursorsOp)

	t.Log("Comparing parsed KillCursors to original KillCursors")
	if !reflect.DeepEqual(killCursorsOp.CursorIds, op.CursorIds) {
		t.Errorf("CursorId Arrays not matched. Saw %v -- Expected %v\n", killCursorsOp.CursorIds, op.CursorIds)
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

	t.Logf("Generated Query: %#v\n", op.QueryOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.QueryOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}
	queryOp := receivedOp.(*mongoproto.QueryOp)

	t.Log("Comparing parsed Query to original Query")
	switch {
	case queryOp.Collection != op.Collection:
		t.Errorf("Collections not equal. Saw %v -- Expected %v\n", queryOp.Collection, op.Collection)
	case !reflect.DeepEqual(&selector, queryOp.Selector):
		t.Errorf("Selectors not equal. Saw %v -- Expected %v\n", queryOp.Selector, &selector)
	case queryOp.Flags != op.Flags:
		t.Errorf("Flags not equal. Saw %d -- Expected %d\n", queryOp.Flags, op.Flags)
	case queryOp.Skip != op.Skip:
		t.Errorf("Skips not equal. Saw %d -- Expected %d\n", queryOp.Skip, op.Skip)
	case queryOp.Limit != op.Limit:
		t.Errorf("Limits not equal. Saw %d -- Expected %d\n", queryOp.Limit, op.Limit)
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

	t.Logf("Generated Update: %#v\n", op.UpdateOp)

	result, err := generator.fetchRecordedOpsFromConn(&op.UpdateOp)
	if err != nil {
		t.Error(err)
	}
	receivedOp, err := result.RawOp.Parse()
	if err != nil {
		t.Error(err)
	}

	updateOp := receivedOp.(*mongoproto.UpdateOp)
	t.Log("Comparing parsed Update to original Update")
	switch {
	case updateOp.Collection != op.Collection:
		t.Errorf("Collections not equal. Saw %v -- Expected %v\n", updateOp.Collection, op.Collection)
	case !reflect.DeepEqual(updateOp.Selector, &selector):
		t.Errorf("Selectors not equal. Saw %v -- Expected %v\n", updateOp.Selector, &selector)
	case !reflect.DeepEqual(updateOp.Update, &update):
		t.Errorf("Updates not equal. Saw %v -- Expected %v\n", updateOp.Update, &update)
	case updateOp.Flags != op.Flags:
		t.Errorf("Flags not equal. Saw %d -- Expected %d\n", updateOp.Flags, op.Flags)
	}
}
