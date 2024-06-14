package txn

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// test each type of transaction individually and serially.
func TestSingleTxnBuffer(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buffer := NewBuffer()
	txnByID, err := mapTestTxnByID()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			testBufferOps(t, buffer, c.ops, txnByID)
		})
	}
}

func TestMixedTxnBuffer(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buffer := NewBuffer()
	txnByID, err := mapTestTxnByID()
	if err != nil {
		t.Fatal(err)
	}

	streams := make([][]db.Oplog, len(testCases))
	for i, c := range testCases {
		streams[i] = c.ops
	}
	ops := testutil.MergeOplogStreams(streams)

	testBufferOps(t, buffer, ops, txnByID)
}

func testBufferOps(t *testing.T, buffer *Buffer, ops []db.Oplog, txnByID map[ID]*TestData) {

	innerOpCounter := make(map[ID]int)

	for _, op := range ops {
		meta, _ := NewMeta(op)

		if !meta.IsTxn() {
			return
		}

		err := buffer.AddOp(meta, op)
		if err != nil {
			t.Fatalf("AddOp failed: %v", err)
		}

		if meta.IsAbort() {
			err := buffer.PurgeTxn(meta)
			if err != nil {
				t.Fatalf("PurgeTxn (abort) failed: %v", err)
			}
			assertNoStateForID(t, meta, buffer)
			continue
		}

		if !meta.IsCommit() {
			continue
		}

		// From here, we're simulating "applying" transaction entries
		ops, errs := buffer.GetTxnStream(meta)

	LOOP:
		for {
			select {
			case _, ok := <-ops:
				if !ok {
					break LOOP
				}
				innerOpCounter[meta.id]++
			case err := <-errs:
				if err != nil {
					t.Fatalf("GetTxnStream streaming failed: %v", err)
				}
				break LOOP
			}
		}

		expectedCnt := txnByID[meta.id].innerOpCount
		if innerOpCounter[meta.id] != expectedCnt {
			t.Errorf("incorrect streamed op count; got %d, expected %d", innerOpCounter[meta.id], expectedCnt)
		}

		err = buffer.PurgeTxn(meta)
		if err != nil {
			t.Fatalf("PurgeTxn (commit) failed: %v", err)
		}
		assertNoStateForID(t, meta, buffer)

	}

}

func assertNoStateForID(t *testing.T, meta Meta, buffer *Buffer) {
	_, ok := buffer.txns[meta.id]
	if ok {
		t.Errorf("state not cleared for %v", meta.id)
	}
}

func TestOldestTimestamp(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buffer := NewBuffer()

	// With no transactions, oldest active is zero value
	oldest := buffer.OldestOpTime()
	zeroTimestamp := primitive.Timestamp{}
	if oldest.Timestamp != zeroTimestamp {
		t.Errorf("expected zero timestamp, but got %v", oldest.Timestamp)
	}

	// Constructing manually requires pointers to int64, so they can't be constants.
	txnN := []int64{0, 1}

	ops := []db.Oplog{
		{
			Timestamp: primitive.Timestamp{T: 1234, I: 1},
			LSID:      bson.Raw{0, 0, 0, 0, 1},
			TxnNumber: &txnN[0],
			Operation: "c",
			Namespace: "admin.$cmd",
			Object: bson.D{
				{"applyOps", bson.A{bson.D{{"op", "n"}}}},
				{"partialTxn", true},
			},
		},
		{
			Timestamp: primitive.Timestamp{T: 1235, I: 1},
			LSID:      bson.Raw{0, 0, 0, 0, 2},
			TxnNumber: &txnN[1],
			Operation: "c",
			Namespace: "admin.$cmd",
			Object: bson.D{
				{"applyOps", bson.A{bson.D{{"op", "n"}}}},
				{"partialTxn", true},
			},
		},
		{
			Timestamp: primitive.Timestamp{T: 1236, I: 1},
			LSID:      bson.Raw{0, 0, 0, 0, 1},
			TxnNumber: &txnN[0],
			Operation: "c",
			Namespace: "admin.$cmd",
			Object: bson.D{
				{"applyOps", bson.A{bson.D{{"op", "n"}}}},
				{"partialTxn", true},
			},
		},
	}

	for _, v := range ops {
		meta, err := NewMeta(v)
		if err != nil {
			t.Fatal(err)
		}
		err = buffer.AddOp(meta, v)
		if err != nil {
			t.Fatal(err)
		}
	}

	// With uncommitted transactions, we should see the oldest among them.
	oldest = buffer.OldestOpTime()
	expect := primitive.Timestamp{T: 1234, I: 1}
	if oldest.Timestamp != expect {
		t.Fatalf("expected timestamp %v, but got %v", expect, oldest)
	}
}

func TestExtractInnerOps(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// Constructing manually requires pointers to int64, so they can't be constants.
	txnN := []int64{0}
	term := []int64{1}
	hash := []int64{2}

	timestamp := primitive.Timestamp{T: 1234, I: 1}

	Convey("extracted oplogs from transaction oplog should have the same timestamp, term and hash", t, func() {
		op := db.Oplog{
			Timestamp: primitive.Timestamp{T: 1234, I: 1},
			Term:      &term[0],
			Hash:      &hash[0],
			LSID:      bson.Raw{0, 0, 0, 0, 1},
			TxnNumber: &txnN[0],
			Operation: "c",
			Namespace: "admin.$cmd",
			Object: bson.D{
				{"applyOps", bson.A{bson.D{{"op", "n"}}}},
				{"partialTxn", true},
			},
		}

		innerOps, err := extractInnerOps(&op)
		if err != nil {
			t.Fatalf("PurgeTxn (abort) failed: %v", err)
		}

		for _, innerOp := range innerOps {
			So(innerOp.Timestamp, ShouldEqual, timestamp)
			So(*innerOp.Term, ShouldEqual, term[0])
			So(*innerOp.Hash, ShouldEqual, hash[0])
		}
	})
}
