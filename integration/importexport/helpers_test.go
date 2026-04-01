package importexport

import (
	"context"
	"os"
	"testing"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/wcwrapper"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func importCollection(
	t *testing.T,
	ns *options.Namespace,
	filePath string,
	ingestOpts mongoimport.IngestOptions,
) error {
	t.Helper()
	toolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	toolOptions.Namespace = ns
	mi, err := mongoimport.New(mongoimport.Options{
		ToolOptions:   toolOptions,
		InputOptions:  &mongoimport.InputOptions{File: filePath, ParseGrace: "stop"},
		IngestOptions: &ingestOpts,
	})
	if err != nil {
		return err
	}
	defer mi.Close()
	_, _, err = mi.ImportDocuments()
	return err
}

func exportCollectionToFile(t *testing.T, ns *options.Namespace) string {
	t.Helper()
	exportFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	require.NoError(t, err)
	exportToolOptions, err := testutil.GetToolOptions()
	require.NoError(t, err)
	exportToolOptions.Namespace = ns
	me, err := mongoexport.New(mongoexport.Options{
		ToolOptions: exportToolOptions,
		OutputFormatOptions: &mongoexport.OutputFormatOptions{
			Type:       "json",
			JSONFormat: "canonical",
		},
		InputOptions: &mongoexport.InputOptions{},
	})
	require.NoError(t, err)
	defer me.Close()
	_, err = me.Export(exportFile)
	require.NoError(t, err)
	require.NoError(t, exportFile.Close())
	return exportFile.Name()
}

func recreateWithValidator(t *testing.T, coll *mongo.Collection, validator any) {
	t.Helper()
	require.NoError(t, coll.Database().Drop(context.Background()))
	require.NoError(
		t,
		coll.Database().CreateCollection(
			context.Background(), coll.Name(), mopt.CreateCollection().SetValidator(validator),
		),
	)
}

func assertValidationError(t *testing.T, err error, msg string) {
	t.Helper()
	var bwe mongo.BulkWriteException
	if assert.ErrorAs(t, err, &bwe, msg) {
		assert.NotEmpty(t, bwe.WriteErrors, "should have at least one write error")
		assert.Equal(t, 121, bwe.WriteErrors[0].Code, "should be DocumentValidationFailure (121)")
	}
}

func newTestClient(t *testing.T, dbName string) *mongo.Client {
	t.Helper()
	ssl := testutil.GetSSLOptions()
	auth := testutil.GetAuthOptions()
	sessionProvider, err := db.NewSessionProvider(options.ToolOptions{
		General: &options.General{},
		SSL:     &ssl,
		Connection: &options.Connection{
			Host: "localhost",
			Port: db.DefaultTestPort,
		},
		Auth:         &auth,
		URI:          &options.URI{},
		Namespace:    &options.Namespace{},
		WriteConcern: wcwrapper.Majority(),
	})
	require.NoError(t, err, "should create session provider")
	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "should get session")
	t.Cleanup(func() {
		_ = client.Database(dbName).Drop(context.Background())
	})
	return client
}
