package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestIsDefaultIdIndex(t *testing.T) {
	cases := []struct {
		Document  IndexDocument
		IsDefault bool
	}{
		{
			Document: IndexDocument{
				Key: bson.D{{"_id", int32(1)}},
			},
			IsDefault: true,
		},
		{
			Document: IndexDocument{
				Key: bson.D{{"_id", 1}},
			},
			IsDefault: true,
		},
		{
			Document: IndexDocument{
				Key: bson.D{{"_id", ""}}, // legacy
			},
			IsDefault: true,
		},
		{
			Document: IndexDocument{
				Key: bson.D{{"_id", "hashed"}},
			},
			IsDefault: false,
		},
	}

	for _, curCase := range cases {
		assert.Equal(
			t,
			curCase.IsDefault,
			curCase.Document.IsDefaultIdIndex(),
			"%+v", curCase.Document,
		)
	}
}

func TestIndexDocumentMarshalPartialFilterExpression(t *testing.T) {
	t.Run("omits partialFilterExpression when nil", func(t *testing.T) {
		indexDoc := IndexDocument{Key: bson.D{{"_id", 1}}}

		bytes, err := bson.Marshal(indexDoc)
		require.NoError(t, err)

		_, err = bson.Raw(bytes).LookupErr("partialFilterExpression")
		assert.Error(t, err, "expected partialFilterExpression to be omitted when nil")
	})

	t.Run("keeps partialFilterExpression when empty document", func(t *testing.T) {
		emptyPartialFilterExpression := bson.D{}
		indexDoc := IndexDocument{
			Key:                     bson.D{{"_id", 1}},
			PartialFilterExpression: &emptyPartialFilterExpression,
		}

		bytes, err := bson.Marshal(indexDoc)
		require.NoError(t, err)

		partialFilterExpressionValue, err := bson.Raw(bytes).LookupErr("partialFilterExpression")
		require.NoError(t, err, "expected partialFilterExpression to be present")

		partialFilterExpressionDoc, ok := partialFilterExpressionValue.DocumentOK()
		require.True(t, ok, "expected partialFilterExpression to marshal as a document")

		elements, err := partialFilterExpressionDoc.Elements()
		require.NoError(t, err)
		assert.Len(
			t,
			elements,
			0,
			"expected partialFilterExpression to marshal as an empty document",
		)
	})

	t.Run(
		"round-trip marshal and unmarshal with empty partialFilterExpression",
		func(t *testing.T) {
			// Create an index document with an empty partial filter expression
			emptyPartialFilterExpression := bson.D{}
			originalDoc := IndexDocument{
				Key:                     bson.D{{"field", int32(1)}},
				PartialFilterExpression: &emptyPartialFilterExpression,
				Options:                 bson.M{"name": "field_1"},
			}

			marshaled, err := bson.Marshal(originalDoc)
			require.NoError(t, err)

			var unmarshaled IndexDocument
			err = bson.Unmarshal(marshaled, &unmarshaled)
			require.NoError(t, err)

			// Verify the empty partial filter expression is preserved
			assert.NotNil(
				t,
				unmarshaled.PartialFilterExpression,
				"expected partialFilterExpression to be non-nil after round-trip",
			)
			assert.Equal(
				t,
				len(*unmarshaled.PartialFilterExpression),
				0,
				"expected empty partialFilterExpression to remain empty",
			)
			assert.Equal(
				t,
				originalDoc.Key,
				unmarshaled.Key,
				"expected key to match after round-trip",
			)
		},
	)
}
