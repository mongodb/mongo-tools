package mongorestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestShardedFullRestore covers the guard in MongoRestore.Restore that refuses a
// restore of a whole sharded system, because a dump taken through mongos
// includes the config database and restoring that would overwrite the cluster's
// own metadata.
func TestShardedFullRestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)

	const (
		dbName   = "sharded_fullrestore"
		collName = "test"
		wantDocs = 100
	)

	session, err := testutil.GetBareSession(t)
	require.NoError(t, err, "can connect to the test cluster")

	coll := session.Database(dbName).Collection(collName)
	require.NoError(t, coll.Database().Drop(context.Background()), "can drop the test database")
	t.Cleanup(func() {
		_ = coll.Database().Drop(context.Background())
	})

	docs := make([]any, 0, wantDocs)
	for range wantDocs {
		docs = append(docs, bson.D{{"x", 1}})
	}
	_, err = coll.InsertMany(context.Background(), docs)
	require.NoError(t, err, "can insert the documents")

	dumpDir := t.TempDir()
	dumpOpts, err := testopts.GetToolOptions()
	require.NoError(t, err, "can build the dump options")
	require.NoError(
		t,
		runDump(t, dumpOpts, dumpDir, nil),
		"a full dump of a sharded system succeeds",
	)

	configDir := filepath.Join(dumpDir, "config")
	_, err = os.Stat(configDir)
	require.NoError(t, err, "the full dump includes the config database")

	require.NoError(t, coll.Database().Drop(context.Background()), "can drop the restore target")

	restoreOpts, err := testopts.GetToolOptions()
	require.NoError(t, err, "can build the restore options")
	require.ErrorContains(
		t,
		runRestore(t, restoreOpts, dumpDir, nil),
		"cannot do a full restore on a sharded system",
		"a full restore of a sharded system is refused",
	)

	require.NoError(t, os.RemoveAll(configDir), "can remove the config dump from the dump")

	restoreOpts, err = testopts.GetToolOptions()
	require.NoError(t, err, "can build the restore options")
	require.NoError(
		t,
		runRestore(t, restoreOpts, dumpDir, nil),
		"a restore of the same dump without its config database succeeds",
	)

	assert.EqualValues(
		t,
		wantDocs,
		docCount(t, coll),
		"every document is restored once the config database is out of the dump",
	)
}
