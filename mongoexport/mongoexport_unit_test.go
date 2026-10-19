// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

//go:build unit

package mongoexport

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestExtendedJSON(t *testing.T) {
	x := bson.M{
		"_id": bson.NewObjectID(),
		"hey": "sup",
		"subdoc": bson.M{
			"subid": bson.NewObjectID(),
		},
		"array": []any{
			bson.NewObjectID(),
			bson.Undefined{},
		},
	}
	out, err := bsonutil.ConvertBSONValueToLegacyExtJSON(x)
	require.NoError(t, err)

	jsonEncoder := json.NewEncoder(os.Stdout)
	err = jsonEncoder.Encode(out)
	require.NoError(t, err)
}

func TestFieldSelect(t *testing.T) {
	assert.Equal(t, bson.M{"_id": 1, "a": 1, "b": 1}, makeFieldSelector("a,b"))
	assert.Equal(t, bson.M{"_id": 1}, makeFieldSelector(""))
	assert.Equal(t, bson.M{"_id": 1, "foo": 1, "x": 1}, makeFieldSelector("x,foo.baz"))
}
