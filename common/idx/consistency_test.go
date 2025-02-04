package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFindInconsistency_OK(t *testing.T) {
	docs := []IndexDocument{
		{
			Options: bson.M{
				"name": "myindex",
			},
			Key: bson.D{
				{"_id", 1},
			},
		},
		{
			Options: bson.M{
				"name":                 "myindex",
				"2dsphereIndexVersion": 2,
			},
			Key: bson.D{
				{"_id", 1},
				{"someField", "2dsphere"},
			},
		},
		{
			Options: bson.M{
				"name":             "myindex",
				"textIndexVersion": 2,
			},
			Key: bson.D{
				{"_id", 1},
				{"someField", "text"},
			},
		},
	}

	for _, idx := range docs {
		assert.NoError(t, idx.FindInconsistency(), "should be OK: %+v", idx)
	}
}

func TestFindInconsistency_UnversionedTextIndex(t *testing.T) {
	idx := IndexDocument{
		Options: bson.M{
			"name": "myindex",
		},
		Key: bson.D{
			{"oneField", 1},
			{"someField", "text"},
		},
	}

	err := idx.FindInconsistency()
	require.Error(t, err)
	assert.ErrorContains(t, err, idx.Options["name"].(string))
	assert.ErrorContains(t, err, "textIndexVersion")
	assert.ErrorContains(t, err, "someField")
}

func TestFindInconsistency_Unversioned2dsphere(t *testing.T) {
	idx := IndexDocument{
		Options: bson.M{
			"name": "myindex",
		},
		Key: bson.D{
			{"oneField", 1},
			{"someField", "2dsphere"},
		},
	}

	err := idx.FindInconsistency()
	require.Error(t, err)
	assert.ErrorContains(t, err, idx.Options["name"].(string))
	assert.ErrorContains(t, err, "2dsphereIndexVersion")
	assert.ErrorContains(t, err, "someField")
}
