package mongorestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestFindEmptyTimestampFields_ShouldFind(t *testing.T) {
	cases := []struct {
		doc    bson.D
		fields [][]string
	}{
		{
			doc: bson.D{
				{"foo", bson.Timestamp{}},
			},
			fields: [][]string{{"foo"}},
		},
		{
			doc: bson.D{
				{"foo", bson.A{
					"123",
					bson.Timestamp{},
				}},
			},
			fields: [][]string{{"foo", "1"}},
		},
		{
			doc: bson.D{
				{"foo", bson.A{
					"123",
					bson.M{"bar": bson.Timestamp{}},
				}},
			},
			fields: [][]string{{"foo", "1", "bar"}},
		},
		{
			doc: bson.D{
				{"foo", bson.A{
					"123",
					bson.M{"bar": bson.Timestamp{}},
					bson.Timestamp{},
				}},
				{"baz", bson.Timestamp{}},
				{"", bson.Timestamp{}},
				{"not", bson.Timestamp{1, 0}},
			},
			fields: [][]string{
				{"foo", "1", "bar"},
				{"foo", "2"},
				{"baz"},
				{""},
			},
		},
	}

	for _, tc := range cases {
		raw, err := bson.Marshal(tc.doc)
		require.NoError(t, err, "must marshal doc: %+v", tc.doc)

		fields, err := FindZeroTimestamps(raw)
		require.NoError(t, err, "should seek empty timestamps in doc %+v", tc.doc)
		assert.ElementsMatch(
			t,
			tc.fields,
			fields,
			"should find empty timestamps in doc %+v",
			tc.doc,
		)
	}
}

func TestFindEmptyTimestampFields_ShouldNotFind(t *testing.T) {
	docs := []bson.D{
		{},
		{{"faux", bson.Binary{
			Data: append(
				[]byte{0x11, 0x42, 0x42, 0x00},
				make([]byte, 8, 8)...,
			),
		}}},
	}

	for _, doc := range docs {
		raw, err := bson.Marshal(doc)
		require.NoError(t, err, "must marshal doc: %+v", doc)

		fields, err := FindZeroTimestamps(raw)
		require.NoError(t, err, "should seek empty timestamps in doc %+v", doc)
		assert.Empty(t, fields, "should find no empty timestamps in doc %+v", doc)
	}
}
