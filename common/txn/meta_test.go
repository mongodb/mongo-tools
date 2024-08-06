// Copyright (C) MongoDB, Inc. 2019-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package txn

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
)

func TestTxnMeta(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	for _, c := range testCases {
		t.Run(c.name, func(*testing.T) {
			if c.notTxn {
				runNonTxnMetaCase(t, c)
			} else {
				runTxnMetaCase(t, c)
			}
		})
	}
}

func runNonTxnMetaCase(t *testing.T, c *TestData) {
	meta, err := NewMeta(c.ops[0])
	if err != nil {
		t.Fatalf("case %s: failed to parse op: %v", c.name, err)
	}

	if meta.IsTxn() {
		t.Errorf("case %s: non-txn meta looks like transaction", c.name)
	}

	return
}

func runTxnMetaCase(t *testing.T, c *TestData) {
	ops := c.ops

	// Double check that we get all the ops we expected.
	if len(ops) != c.entryCount {
		t.Errorf("case %s: expected %d ops, but got %d", c.name, c.entryCount, len(ops))
	}

	// Test properties of each op.
	for i, o := range ops {
		meta, err := NewMeta(o)
		if err != nil {
			t.Fatalf("case %s [%d]: failed to parse op: %v", c.name, i, err)
		}

		if (meta.id == ID{}) {
			t.Errorf("case %s [%d]: Id was zero value", c.name, i)
		}

		isMultiOp := c.entryCount > 1
		if meta.IsMultiOp() != isMultiOp {
			t.Errorf(
				"case %s [%d]: expected IsMultiOp %v, but got %v",
				c.name,
				i,
				meta.IsMultiOp(),
				isMultiOp,
			)
		}

		if i == 0 {
			if !meta.IsData() {
				t.Errorf("case %s [%d]: op should have parsed as data, but it wasn't", c.name, i)
			}
		}

		if i != len(ops)-1 {
			if meta.IsFinal() {
				t.Errorf("case %s [%d]: op parsed as final, but it wasn't", c.name, i)
			}
		}
	}

	// Test properties of the last op.
	lastOp := ops[len(ops)-1]
	meta, _ := NewMeta(lastOp)

	if !meta.IsFinal() {
		t.Errorf("case %s: last oplog entry not marked final", c.name)
	}

	if c.commits && !meta.IsCommit() {
		t.Errorf("case %s: expected last oplog entry to be a commit but it wasn't", c.name)
	}
	if c.aborts && !meta.IsAbort() {
		t.Errorf("case %s: expected last oplog entry to be a abort but it wasn't", c.name)
	}
}
