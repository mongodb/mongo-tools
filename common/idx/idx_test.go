package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
