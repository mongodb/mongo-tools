// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package bsonutil

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestConvertObjectIdBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	bsonObjId := bson.NewObjectID()
	jsonObjId := json.ObjectId(bsonObjId.Hex())

	_jObjId, err := ConvertBSONValueToLegacyExtJSON(bsonObjId)
	require.NoError(t, err)
	jObjId, ok := _jObjId.(json.ObjectId)
	assert.True(t, ok)

	assert.NotEqual(t, bsonObjId, jObjId)
	assert.Equal(t, jsonObjId, jObjId)
}

func TestArraysBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("should work for empty arrays", func(t *testing.T) {
		jArr, err := ConvertBSONValueToLegacyExtJSON([]any{})
		require.NoError(t, err)
		assert.Equal(t, []any{}, jArr)
	})

	t.Run("should work for one-level deep arrays", func(t *testing.T) {
		objId := bson.NewObjectID()
		bsonArr := []any{objId, 28, 0.999, "plain"}
		_jArr, err := ConvertBSONValueToLegacyExtJSON(bsonArr)
		require.NoError(t, err)
		jArr, ok := _jArr.([]any)
		assert.True(t, ok)

		assert.Len(t, jArr, 4)
		assert.Equal(t, json.ObjectId(objId.Hex()), jArr[0])
		assert.EqualValues(t, 28, jArr[1])
		assert.EqualValues(t, 0.999, jArr[2])
		assert.Equal(t, "plain", jArr[3])
	})

	t.Run("should work for arrays with embedded objects", func(t *testing.T) {
		bsonObj := []any{
			80,
			bson.M{
				"a": int64(20),
				"b": bson.M{
					"c": bson.Regex{Pattern: "hi", Options: "i"},
				},
			},
		}

		_JObj, err := ConvertBSONValueToLegacyExtJSON(bsonObj)
		require.NoError(t, err)
		_jObj, ok := _JObj.([]any)
		assert.True(t, ok)
		jObj, ok := _jObj[1].(bson.M)
		assert.True(t, ok)
		assert.Len(t, jObj, 2)
		assert.Equal(t, json.NumberLong(20), jObj["a"])
		jjObj, ok := jObj["b"].(bson.M)
		assert.True(t, ok)

		assert.Equal(t, json.RegExp{"hi", "i"}, jjObj["c"])
		assert.NotEqual(t, json.RegExp{"i", "hi"}, jjObj["c"])
	})

}

func TestDateBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	timeNow := time.Now()
	secs := timeNow.Unix()
	nanosecs := timeNow.Nanosecond()
	millis := int64(nanosecs / 1e6)

	timeNowSecs := time.Unix(secs, 0)
	timeNowMillis := time.Unix(secs, millis*1e6)

	// json.Date is stored as an int64 representing the number of milliseconds since the epoch
	t.Run(fmt.Sprintf("second granularity: %v", timeNowSecs), func(t *testing.T) {
		_jObj, err := ConvertBSONValueToLegacyExtJSON(timeNowSecs)
		require.NoError(t, err)
		jObj, ok := _jObj.(json.Date)
		assert.True(t, ok)

		assert.Equal(t, secs*1e3, int64(jObj))
	})

	t.Run(fmt.Sprintf("millisecond granularity: %v", timeNowMillis), func(t *testing.T) {
		_jObj, err := ConvertBSONValueToLegacyExtJSON(timeNowMillis)
		require.NoError(t, err)
		jObj, ok := _jObj.(json.Date)
		assert.True(t, ok)

		assert.Equal(t, secs*1e3+millis, int64(jObj))
	})

	t.Run(fmt.Sprintf("nanosecond granularity: %v", timeNow), func(t *testing.T) {
		_jObj, err := ConvertBSONValueToLegacyExtJSON(timeNow)
		require.NoError(t, err)
		jObj, ok := _jObj.(json.Date)
		assert.True(t, ok)

		// we lose nanosecond precision
		assert.Equal(t, secs*1e3+millis, int64(jObj))
	})
}

func TestMaxKeyBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(bson.MaxKey{})
	require.NoError(t, err)
	jObj, ok := _jObj.(json.MaxKey)
	assert.True(t, ok)

	assert.Equal(t, json.MaxKey{}, jObj, "produce a json.MaxKey")
}

func TestMinKeyBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(bson.MinKey{})
	require.NoError(t, err)
	jObj, ok := _jObj.(json.MinKey)
	assert.True(t, ok)

	assert.Equal(t, json.MinKey{}, jObj)
}

func Test64BitIntBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(int32(243))
	require.NoError(t, err)
	jObj, ok := _jObj.(json.NumberInt)
	assert.True(t, ok)

	assert.Equal(t, json.NumberInt(243), jObj)
}

func Test32BitIntBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(int64(888234334343))
	require.NoError(t, err)
	jObj, ok := _jObj.(json.NumberLong)
	assert.True(t, ok)

	assert.Equal(t, json.NumberLong(888234334343), jObj)
}

func TestRegExBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(bson.Regex{"decision", "gi"})
	require.NoError(t, err)
	jObj, ok := _jObj.(json.RegExp)
	assert.True(t, ok)

	assert.Equal(t, json.RegExp{"decision", "gi"}, jObj)
}

func TestUndefinedValueBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(bson.Undefined{})
	require.NoError(t, err)
	jObj, ok := _jObj.(json.Undefined)
	assert.True(t, ok)

	assert.Equal(t, json.Undefined{}, jObj)
}

func TestTimestampBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	// {t:803434343, i:9} == bson.MongoTimestamp(803434343*2**32 + 9)
	_jObj, err := ConvertBSONValueToLegacyExtJSON(bson.Timestamp{T: 803434343, I: 9})
	require.NoError(t, err)
	jObj, ok := _jObj.(json.Timestamp)
	assert.True(t, ok)

	assert.Equal(t, json.Timestamp{Seconds: 803434343, Increment: 9}, jObj)
	assert.NotEqual(t, json.Timestamp{Seconds: 803434343, Increment: 8}, jObj)
}

func TestBinaryBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON(
		bson.Binary{'\x01', []byte("\x05\x20\x02\xae\xf7")},
	)
	require.NoError(t, err)
	jObj, ok := _jObj.(json.BinData)
	assert.True(t, ok)

	base64data1 := base64.StdEncoding.EncodeToString([]byte("\x05\x20\x02\xae\xf7"))
	base64data2 := base64.StdEncoding.EncodeToString([]byte("\x05\x20\x02\xaf\xf7"))
	assert.Equal(t, json.BinData{'\x01', base64data1}, jObj)
	assert.NotEqual(t, json.BinData{'\x01', base64data2}, jObj)
}

func TestGenericBytesBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_jObj, err := ConvertBSONValueToLegacyExtJSON([]byte("this is something that's cool"))
	require.NoError(t, err)
	jObj, ok := _jObj.(json.BinData)
	assert.True(t, ok)

	base64data := base64.StdEncoding.EncodeToString([]byte("this is something that's cool"))
	assert.Equal(t, json.BinData{0x00, base64data}, jObj)
	assert.NotEqual(t, json.BinData{0x01, base64data}, jObj)
}

func TestUnknownBSONTypeToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	_, err := ConvertBSONValueToLegacyExtJSON(func() {})
	require.Error(t, err)
}

func TestDBPointerBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	objId := bson.NewObjectID()
	_jObj, err := ConvertBSONValueToLegacyExtJSON(
		bson.DBPointer{"dbrefnamespace", objId},
	)
	require.NoError(t, err)
	jObj, ok := _jObj.(json.DBPointer)
	assert.True(t, ok)

	assert.Equal(t, json.DBPointer{"dbrefnamespace", objId}, jObj)
}

func TestJSCodeBSONToJSON(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("without scope", func(t *testing.T) {
		_jObj, err := ConvertBSONValueToLegacyExtJSON(
			bson.CodeWithScope{"function() { return null; }", nil},
		)
		require.NoError(t, err)
		jObj, ok := _jObj.(json.JavaScript)
		assert.True(t, ok)

		assert.Equal(t, json.JavaScript{"function() { return null; }", nil}, jObj)
	})

	t.Run("with scope", func(t *testing.T) {
		_jObj, err := ConvertBSONValueToLegacyExtJSON(
			bson.CodeWithScope{"function() { return x; }", bson.M{"x": 2}},
		)
		require.NoError(t, err)
		jObj, ok := _jObj.(json.JavaScript)
		assert.True(t, ok)

		scopeMap, ok := jObj.Scope.(bson.M)
		assert.True(t, ok)

		assert.EqualValues(t, 2, scopeMap["x"])
		assert.Equal(t, "function() { return x; }", jObj.Code)
	})
}
