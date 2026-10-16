// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongodump

import (
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestErrorOnImportCollection(t *testing.T) {
	rawOp, err := os.ReadFile("./testdata/importCollection.bson")
	require.NoError(t, err)

	err = oplogDocumentValidator(rawOp)
	require.Error(t, err)
	assert.EqualError(t, err, "cannot dump with oplog while importCollection occurs")
}

func TestErrorOnViewlessTimeseriesConversion(t *testing.T) {
	rawOp, err := bson.Marshal(db.Oplog{
		Timestamp: bson.Timestamp{T: 1, I: 1},
		Version:   2,
		Operation: "c",
		Namespace: "test.$cmd",
		Object: bson.D{
			{"upgradeDowngradeViewlessTimeseries", "foo_ts"},
			{"isUpgrade", true},
		},
	})
	require.NoError(t, err)

	err = oplogDocumentValidator(rawOp)
	assert.EqualError(
		t,
		err,
		"cannot dump with oplog while a timeseries format conversion caused by an FCV "+
			"change between 8.x and 9.0 occurs",
		"a dump whose oplog window spans a viewless timeseries conversion is rejected",
	)
}
