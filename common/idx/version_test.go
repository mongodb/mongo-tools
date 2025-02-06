package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestEnsureIndexVersions(t *testing.T) {
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
		assert.Empty(t, idx.EnsureIndexVersions(), "should infer nothing: %+v", idx)
	}
}

func TestEnsureIndexVersions_UnversionedTextIndex(t *testing.T) {
	idx := IndexDocument{
		Options: bson.M{
			"name": "myindex",
		},
		Key: bson.D{
			{"oneField", 1},
			{"someField", "text"},
		},
	}

	inferred := idx.EnsureIndexVersions()
	assert.Equal(t, map[string]any{"textIndexVersion": 1}, inferred)
	assert.Equal(t, 1, idx.Options["textIndexVersion"])
}

func TestEnsureIndexVersions_Unversioned2dsphere(t *testing.T) {
	idx := IndexDocument{
		Options: bson.M{
			"name": "myindex",
		},
		Key: bson.D{
			{"oneField", 1},
			{"someField", "2dsphere"},
		},
	}

	inferred := idx.EnsureIndexVersions()
	assert.Equal(t, map[string]any{"2dsphereIndexVersion": 1}, inferred)
	assert.Equal(t, 1, idx.Options["2dsphereIndexVersion"])
}
