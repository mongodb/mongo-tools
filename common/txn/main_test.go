package txn

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	OplogEntriesFile = "testdata/oplog_entries.json"
)

func readTestData() (bson.Raw, error) {
	b, err := os.ReadFile(OplogEntriesFile)
	if err != nil {
		return nil, fmt.Errorf("Couldn't load %s: %v", OplogEntriesFile, err)
	}
	var data bson.Raw
	err = bson.UnmarshalExtJSON(b, false, &data)
	if err != nil {
		return nil, fmt.Errorf("Couldn't decode JSON: %v", err)
	}
	return data, nil
}

func getOpsForCase(name string, data bson.Raw) ([]db.Oplog, error) {
	rawArray, err := data.LookupErr(name, "ops")
	if err != nil {
		return nil, fmt.Errorf("Couldn't find ops for case %s: %v", name, err)
	}
	rawOps, err := rawArray.Array().Elements()
	if err != nil {
		return nil, fmt.Errorf("Couldn't extract array elements for case %s: %v", name, err)
	}
	ops := make([]db.Oplog, len(rawOps))
	for i, e := range rawOps {
		err := e.Value().Unmarshal(&ops[i])
		if err != nil {
			return nil, fmt.Errorf("Couldn't unmarshal op %d for case %s: %v", i, name, err)
		}
	}
	return ops, nil
}

func mapTestTxnByID() (map[ID]*TestData, error) {
	m := make(map[ID]*TestData)
	for _, c := range testCases {
		meta, err := NewMeta(c.ops[0])
		if err != nil {
			return nil, err
		}
		if meta.IsTxn() {
			m[meta.id] = c
		}
	}
	return m, nil
}

type TestData struct {
	name         string
	entryCount   int
	innerOpCount int
	notTxn       bool
	commits      bool
	aborts       bool
	ops          []db.Oplog
}

var testCases = []*TestData{
	{name: "not transaction", entryCount: 1, notTxn: true},
	{name: "applyops not transaction", entryCount: 1, notTxn: true},
	{name: "small, unprepared", entryCount: 1, innerOpCount: 3, commits: true},
	{name: "large, unprepared", entryCount: 3, innerOpCount: 6, commits: true},
	{name: "small, prepared, committed", entryCount: 2, innerOpCount: 4, commits: true},
	{name: "small, prepared, aborted", entryCount: 2, innerOpCount: 5, aborts: true},
	{name: "large, prepared, committed", entryCount: 4, innerOpCount: 10, commits: true},
	{name: "large, prepared, aborted", entryCount: 4, innerOpCount: 9, aborts: true},
	{name: "not transaction with lsid", entryCount: 1, notTxn: true},
	{name: "not transaction with lsid and txnNumber", entryCount: 1, notTxn: true},
	{name: "not transaction with lsid and txnNumber and command", entryCount: 1, notTxn: true},
}

func TestMain(m *testing.M) {
	flag.Parse()

	data, err := readTestData()
	if err != nil {
		panic(err)
	}

	for i, c := range testCases {
		ops, err := getOpsForCase(c.name, data)
		if err != nil {
			panic(err)
		}
		// c is a copy and we want to change the original
		testCases[i].ops = ops
	}

	os.Exit(m.Run())
}
