package legacy_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDumpRestore1(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	ctx := context.Background()

	session, err := testutil.GetBareSession()
	require.NoError(t, err, "should connect to server")

	// Setup test database
	dbName := "foo"
	collName := "foo"
	db := session.Database(dbName)
	coll := db.Collection(collName)

	// Ensure the collection is empty
	err = coll.Drop(ctx)
	require.NoError(t, err)

	// Insert one document
	_, err = coll.InsertOne(ctx, bson.M{"a": 22})
	require.NoError(t, err)

	// Verify one document inserted
	count, err := coll.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	// Create temp dir for dump files
	dumpDir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()

	// Run mongodump
	args := append(testutil.GetBareArgs(), "--out", dumpDir)
	cmd := exec.Command("mongodump", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "mongodump failed: %s", output)

	// Drop collection to verify restore works
	err = coll.Drop(ctx)
	require.NoError(t, err)

	// Verify collection is empty
	count, err = coll.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	// Run mongorestore
	args = append([]string{"mongorestore"}, append(testutil.GetBareArgs(), "--dir", dumpDir)...)
	cmd = exec.Command(args[0], args[1:]...)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "mongorestore failed: %s", output)

	// Verify document was restored
	var doc bson.M
	err = coll.FindOne(ctx, bson.M{}).Decode(&doc)
	require.NoError(t, err, "should find restored document")
	require.Equal(t, int32(22), doc["a"])
}

// Test that mongodump returns failure when --collection is used without --db

func TestMongodumpFailsWithoutDB(t *testing.T) {
	args := append(testutil.GetBareArgs(), "--collection", "col")
	cmd := exec.Command("mongodump", args...)
	_, err := cmd.CombinedOutput()
	require.Error(t, err, "mongodump should fail when --collection is used without --db")
}

// Ensure that --db and --collection are provided when filename is "-" (stdin)
func TestMongorestoreFailsWithoutDBForStdin(t *testing.T) {
	args := []string{"mongorestore", "--collection", "coll", "--dir", "-"}
	cmd := exec.Command(args[0], args[1:]...)
	_, err := cmd.CombinedOutput()
	require.Error(t, err, "mongorestore should fail when --collection is provided without --db for stdin")
}

func TestMongorestoreFailsWithoutCollectionForStdin(t *testing.T) {
	args := []string{"mongorestore", "--db", "db", "--dir", "-"}
	cmd := exec.Command(args[0], args[1:]...)
	_, err := cmd.CombinedOutput()
	require.Error(t, err, "mongorestore should fail when --db is provided without --collection for stdin")
}
