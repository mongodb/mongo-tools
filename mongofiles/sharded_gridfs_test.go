package mongofiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestShardedGridFS puts a file large enough to span many GridFS chunks through
// mongos and reads it back, for each of the ways fs.chunks can be laid out on a
// sharded cluster.
func TestShardedGridFS(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)

	sessionProvider, err := db.NewSessionProvider(*toolOptions)
	require.NoError(t, err, "can build a session provider for the cluster")
	defer sessionProvider.Close()

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "can connect to the cluster")

	localFile := writeLargeLocalFile(t)

	cases := []struct {
		name        string
		dbName      string
		shardDB     bool
		shardKey    bson.D
		uniqueChunk bool
	}{
		{name: "unsharded", dbName: "gridfs_unsharded"},
		{name: "sharded db with unsharded collection", dbName: "gridfs_sharded_db", shardDB: true},
		{
			name:     "fs.chunks sharded on files_id",
			dbName:   "gridfs_sharded_files_id",
			shardDB:  true,
			shardKey: bson.D{{"files_id", 1}},
		},
		{
			name:        "fs.chunks sharded on files_id and n",
			dbName:      "gridfs_sharded_files_id_n",
			shardDB:     true,
			shardKey:    bson.D{{"files_id", 1}, {"n", 1}},
			uniqueChunk: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := client.Database(c.dbName)
			require.NoError(t, database.Drop(context.Background()), "can drop the test database")
			t.Cleanup(func() {
				_ = database.Drop(context.Background())
			})

			if c.shardDB {
				enableSharding(t, client, c.dbName)
			}
			if c.shardKey != nil {
				shardCollection(t, client, c.dbName+".fs.chunks", c.shardKey, c.uniqueChunk)
			}

			filesColl := database.Collection("fs.files")

			runShardedMongofiles(t, c.dbName, "put", func(mf *MongoFiles) {
				mf.StorageOptions.LocalFileName = localFile
			})

			assert.EqualValues(
				t,
				1,
				countIn(t, filesColl),
				"the file is stored as a single GridFS file",
			)
			assert.Greater(
				t,
				countIn(t, database.Collection("fs.chunks")),
				int64(1),
				"the file is large enough to be stored as more than one chunk",
			)

			downloaded := filepath.Join(t.TempDir(), "downloaded")
			runShardedMongofiles(t, c.dbName, "get", func(mf *MongoFiles) {
				mf.StorageOptions.LocalFileName = downloaded
			})

			assert.Equal(
				t,
				fileSHA256(t, localFile),
				fileSHA256(t, downloaded),
				"the downloaded file has the same contents as the original",
			)
		})
	}
}

// largeLocalFileSize is several times the default GridFS chunk size of 255 KiB,
// so that the file is stored as many GridFS chunks, as the file gridfs.js uploaded
// was. Whether those chunks end up on more than one shard is up to the balancer
// and the cluster's chunk size, neither of which this test controls.
const largeLocalFileSize = 4 * 1024 * 1024

// writeLargeLocalFile stands in for the mongod binary that gridfs.js uploaded:
// the point is only that the file is too large to fit in one GridFS chunk.
func writeLargeLocalFile(t *testing.T) string {
	t.Helper()

	contents := make([]byte, largeLocalFileSize)
	_, err := rand.Read(contents)
	require.NoError(t, err, "can generate the contents of the file to upload")

	path := filepath.Join(t.TempDir(), "large-file")
	require.NoError(t, os.WriteFile(path, contents, 0600), "can write the file to upload")

	return path
}

// gridFSName is the name the file is stored under in GridFS. mongofiles refuses
// to write a downloaded file to a name that escapes the current directory, so the
// stored name has to be a plain one rather than the path of the local file.
const gridFSName = "large-file"

func runShardedMongofiles(
	t *testing.T,
	dbName, command string,
	configure func(*MongoFiles),
) {
	t.Helper()

	mf, err := simpleMongoFilesInstanceWithFilename(command, gridFSName)
	require.NoError(t, err, "can build a mongofiles instance for %s", command)
	mf.StorageOptions.DB = dbName
	if configure != nil {
		configure(mf)
	}

	_, err = mf.Run(false)
	require.NoError(t, err, "mongofiles %s succeeds against a sharded cluster", command)
}

func enableSharding(t *testing.T, client *mongo.Client, dbName string) {
	t.Helper()

	err := client.Database("admin").
		RunCommand(context.Background(), bson.D{{"enableSharding", dbName}}).
		Err()
	require.NoError(t, err, "can enable sharding on %s", dbName)
}

func shardCollection(t *testing.T, client *mongo.Client, ns string, key bson.D, unique bool) {
	t.Helper()

	err := client.Database("admin").RunCommand(context.Background(), bson.D{
		{"shardCollection", ns},
		{"key", key},
		{"unique", unique},
	}).Err()
	require.NoError(t, err, "can shard %s on %v", ns, key)
}

func countIn(t *testing.T, coll *mongo.Collection) int64 {
	t.Helper()

	count, err := coll.CountDocuments(context.Background(), bson.D{})
	require.NoError(t, err, "can count the documents in %s", coll.Name())

	return count
}

func fileSHA256(t *testing.T, path string) []byte {
	t.Helper()

	contents, err := os.ReadFile(path)
	require.NoError(t, err, "can read %s", path)

	sum := sha256.Sum256(contents)

	return sum[:]
}
