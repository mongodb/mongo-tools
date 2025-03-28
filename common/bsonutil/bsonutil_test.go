package bsonutil

import (
	"math"
	"testing"
	"time"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBson2Float64(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	assert := assert.New(t)

	decimalVal, _ := primitive.ParseDecimal128("-1")
	tests := []struct {
		in          interface{}
		expected    float64
		isSuccess   bool
		description string
	}{
		{int32(1), 1.0, true, "int32"},
		{int64(1), 1.0, true, "int64"},
		{1.0, 1.0, true, "float"},
		{decimalVal, float64(-1), true, "decimal128"},
		{"invalid value", 0, false, "invalid float value"},
	}

	for _, test := range tests {
		result, ok := Bson2Float64(test.in)
		if test.isSuccess {
			assert.True(ok, "%s converted to float64", test.description)
		} else {
			assert.False(ok, "%s did not convert to float64", test.description)
		}
		assert.Equal(test.expected, result, test.description)
	}
}

// It'd be good to test the case where IsEqual returns an error, but it's not
// clear if this can actually happen in practice. Internally, these errors can
// only occur when the call to `bson.Marshal()` fails. But the type signature
// for IsEqual means that we are always passing `bson.D` values to
// `bson.Marshal()`, and I don't think those can cause marshaling errors.
func TestIsEqual(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	assert := assert.New(t)

	tests := []struct {
		left        bson.D
		right       bson.D
		isEqual     bool
		description string
	}{
		{
			bson.D{{"hello", int64(42)}},
			bson.D{{"hello", int64(42)}},
			true,
			"identical documents with int64 keys",
		},
		{
			bson.D{{"hello", int64(42)}},
			bson.D{{"hello", int32(42)}},
			false,
			"documents have same keys but values have different types",
		},
		{
			bson.D{{"foo", "bar"}, {"baz", "buz"}},
			bson.D{{"baz", "buz"}, {"foo", "bar"}},
			false,
			"document has same key/value pairs but in different order",
		},
		{
			bson.D{{"hello", primitive.DateTime(42)}},
			bson.D{{"hello", primitive.DateTime(42)}},
			true,
			"identical documents with datetime keys",
		},
		{
			bson.D{{"hello", primitive.DateTime(42)}},
			bson.D{{"hello", primitive.DateTime(999)}},
			false,
			"same key but different datetime value",
		},
	}
	for _, test := range tests {
		isEq, err := IsEqual(test.left, test.right)
		if assert.NoError(err) {
			if test.isEqual {
				assert.True(isEq, test.description)
			} else {
				assert.False(isEq)
			}
		}
	}
}

func TestMarshalExtJSONReversible(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		val          any
		reversible   bool
		expectedJSON string
	}{
		{
			bson.M{"field1": bson.M{"$date": 1257894000000}},
			true,
			`{"field1":{"$date":{"$numberLong":"1257894000000"}}}`,
		},
		{
			bson.M{"field1": time.Unix(1257894000, 0)},
			true,
			`{"field1":{"$date":{"$numberLong":"1257894000000"}}}`,
		},
		{
			bson.M{"field1": bson.M{"$date": "invalid"}},
			false,
			``,
		},
	}

	for _, test := range tests {
		json, err := MarshalExtJSONReversible(
			test.val,
			true,  /* canonical */
			false, /* escapeHTML */
		)
		if !test.reversible {
			assert.ErrorContains(t, err, "marshal is not reversible")
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, test.expectedJSON, string(json))
	}
}

func TestMarshalExtJSONWithBSONRoundtripConsistency(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	tests := []struct {
		val                          any
		consistentAfterRoundtripping bool
		expectedJSON                 string
	}{
		{
			bson.M{"field1": bson.M{"grapes": int64(123)}},
			true,
			`{"field1":{"grapes":{"$numberLong":"123"}}}`,
		},
		{
			bson.M{"field1": bson.M{"$date": 1257894000000}},
			false,
			``,
		},
		{
			bson.M{"field1": bson.M{"nanField": math.NaN()}},
			true,
			`{"field1":{"nanField":{"$numberDouble":"NaN"}}}`,
		},
	}

	for _, test := range tests {
		json, err := MarshalExtJSONWithBSONRoundtripConsistency(
			test.val,
			true,  /* canonical */
			false, /* escapeHTML */
		)
		if !test.consistentAfterRoundtripping {
			assert.ErrorContains(
				t,
				err,
				"marshaling BSON to ExtJSON and back resulted in discrepancies",
			)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, test.expectedJSON, string(json))
	}
}
