package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// This sets up a timeseries collection and inserts 1000 logical documents into 10 bucket documents.
// The timeField is 'ts', the metaField  is 'my_meta', and there's an index on 'my_meta.device'.
func SetUpTimeseries(t *testing.T, dbName string, colName string) {
	sessionProvider, _, err := GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	timeseriesOptions := bson.D{
		{"timeField", "ts"},
		{"metaField", "my_meta"},
	}
	createCmd := bson.D{
		{"create", colName},
		{"timeseries", timeseriesOptions},
	}
	var r2 bson.D
	err = sessionProvider.Run(createCmd, &r2, dbName)
	require.NoError(t, err, "create timeseries coll")

	coll := sessionProvider.DB(dbName).Collection(colName)

	idx := mongo.IndexModel{
		Keys: bson.D{{"my_meta.device", 1}},
	}
	_, err = coll.Indexes().CreateOne(t.Context(), idx)
	require.NoError(t, err, "create index 1")

	idx = mongo.IndexModel{
		Keys: bson.D{{"ts", 1}, {"my_meta.device", 1}},
	}
	_, err = coll.Indexes().CreateOne(t.Context(), idx)
	require.NoError(t, err, "create index 2")

	for i := range 1000 {
		metadata := bson.M{
			"device": i % 10,
		}
		_, err = coll.InsertOne(
			t.Context(),
			bson.M{
				"ts":          bson.NewDateTimeFromTime(time.Now()),
				"my_meta":     metadata,
				"measurement": i,
			},
		)

		require.NoError(t, err, "insert ts data (%d)", i)
	}
}
