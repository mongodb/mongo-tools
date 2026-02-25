package txn

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// test each type of transaction individually and serially.
func TestSingleTxnBuffer(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buffer := NewBuffer()
	txnByID, err := mapTestTxnByID()
	require.NoError(t, err)

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
	require.NoError(t, err)

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
		require.NoError(t, err, "AddOp failed")

		if meta.IsAbort() {
			err := buffer.PurgeTxn(meta)
			require.NoError(t, err, "PurgeTxn (abort) failed")
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
				require.NoError(t, err, "GetTxnStream streaming failed")
				break LOOP
			}
		}

		expectedCnt := txnByID[meta.id].innerOpCount
		assert.Equal(t, expectedCnt, innerOpCounter[meta.id], "incorrect streamed op count")

		err = buffer.PurgeTxn(meta)
		require.NoError(t, err, "PurgeTxn (commit) failed")
		assertNoStateForID(t, meta, buffer)
	}
}

func assertNoStateForID(t *testing.T, meta Meta, buffer *Buffer) {
	_, ok := buffer.txns[meta.id]
	assert.False(t, ok, "state not cleared for %v", meta.id)
}

func TestOldestTimestamp(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	buffer := NewBuffer()

	// With no transactions, oldest active is zero value
	oldest := buffer.OldestOpTime()
	assert.Zero(t, oldest.Timestamp)

	// Constructing manually requires pointers to int64, so they can't be constants.
	txnN := []int64{0, 1}

	ops := []db.Oplog{
		{
			Timestamp: bson.Timestamp{T: 1234, I: 1},
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
			Timestamp: bson.Timestamp{T: 1235, I: 1},
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
			Timestamp: bson.Timestamp{T: 1236, I: 1},
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
		require.NoError(t, err)

		err = buffer.AddOp(meta, v)
		require.NoError(t, err)
	}

	// With uncommitted transactions, we should see the oldest among them.
	oldest = buffer.OldestOpTime()
	expect := bson.Timestamp{T: 1234, I: 1}
	require.Equal(t, expect, oldest.Timestamp)
}

func TestExtractInnerOps(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// Constructing manually requires pointers to int64, so they can't be constants.
	txnN := int64(0)
	term := int64(1)

	timestamp := bson.Timestamp{T: 1234, I: 1}

	op := db.Oplog{
		Timestamp: bson.Timestamp{T: 1234, I: 1},
		Term:      &term,
		LSID:      bson.Raw{0, 0, 0, 0, 1},
		TxnNumber: &txnN,
		Operation: "c",
		Namespace: "admin.$cmd",
		Object: bson.D{
			{"applyOps", bson.A{bson.D{{"op", "n"}}}},
			{"partialTxn", true},
		},
	}

	innerOps, err := extractInnerOps(&op)
	require.NoError(t, err, "PurgeTxn (abort) failed")

	for _, innerOp := range innerOps {
		assert.Equal(t, timestamp, innerOp.Timestamp)
		assert.Equal(t, term, *innerOp.Term)
	}
}
