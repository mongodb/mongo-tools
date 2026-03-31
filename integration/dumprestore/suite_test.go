package dumprestore

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type DumpRestoreSuite struct {
	sharedsuite.IntegrationSuite
}

func TestDumpRestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	suite.Run(t, new(DumpRestoreSuite))
}

func (s *DumpRestoreSuite) withBSONMongodump(testCase func(string), args ...string) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	dirArgs := []string{
		"--out", dir,
	}
	s.runMongodumpWithArgs(append(dirArgs, args...)...)
	testCase(dir)
}

func (s *DumpRestoreSuite) runMongodumpWithArgs(args ...string) {
	cmd := []string{"go", "run", filepath.Join("..", "..", "mongodump", "main")}
	cmd = append(cmd, testutil.GetBareArgs()...)
	cmd = append(cmd, args...)
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	cmdStr := strings.Join(cmd, " ")
	s.Require().NoError(err, "can execute command %s with output: %s", cmdStr, out)
	s.Require().NotContains(
		string(out),
		"does not exist",
		"running [%s] does not tell us the namespace does not exist",
		cmdStr,
	)

	// So we can see dump's output when debugging test failures:
	fmt.Print(string(out))
}

func (s *DumpRestoreSuite) createCollectionsWithTestDocuments(
	db *mongo.Database,
	collectionNames []string,
) []*mongo.Collection {
	collections := []*mongo.Collection{}
	for _, collectionName := range collectionNames {
		collection := s.createCollectionWithTestDocument(db, collectionName)
		collections = append(collections, collection)
	}
	return collections
}

func (s *DumpRestoreSuite) createCollectionWithTestDocument(
	db *mongo.Database,
	collectionName string,
) *mongo.Collection {
	collection := db.Collection(collectionName)
	_, err := collection.InsertOne(
		s.Context(),
		testDocument,
	)
	s.Require().NoError(err, "can insert documents into collection")
	return collection
}

func (s *DumpRestoreSuite) clearDB(db *mongo.Database) {
	collectionNames, err := db.ListCollectionNames(s.Context(), bson.D{})
	s.Require().NoError(err, "can get collection names")
	for _, collectionName := range collectionNames {
		collection := db.Collection(collectionName)
		_, _ = collection.DeleteMany(s.Context(), bson.M{})
	}
}

func (s *DumpRestoreSuite) withBSONMongodumpForCollection(
	db string,
	collection string,
	testCase func(string),
) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	s.runBSONMongodumpForCollection(dir, db, collection)
	testCase(dir)
}

func (s *DumpRestoreSuite) runBSONMongodumpForCollection(
	dir, db, collection string,
	args ...string,
) string {
	baseArgs := []string{
		"--out", dir,
		"--db", db,
		"--collection", collection,
	}
	s.runMongodumpWithArgs(
		append(baseArgs, args...)...,
	)
	bsonFile := filepath.Join(dir, db, fmt.Sprintf("%s.bson", collection))
	_, err := os.Stat(bsonFile)
	s.Require().NoError(err, "dump created BSON data file")
	_, err = os.Stat(filepath.Join(dir, db, fmt.Sprintf("%s.metadata.json", collection)))
	s.Require().NoError(err, "dump created JSON metadata file")
	return bsonFile
}

func (s *DumpRestoreSuite) withArchiveMongodump(testCase func(string), dumpArgs ...string) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	file := filepath.Join(dir, "archive")
	s.runArchiveMongodump(file, dumpArgs...)
	testCase(file)
}

func (s *DumpRestoreSuite) runArchiveMongodump(file string, dumpArgs ...string) {
	s.runMongodumpWithArgs(
		append(
			[]string{mongorestore.ArchiveOption + "=" + file},
			dumpArgs...,
		)...,
	)
	_, err := os.Stat(file)
	s.Require().NoError(err, "dump created archive data file")
}

func (s *DumpRestoreSuite) timeseriesBucketsMayHaveMixedSchemaData(
	bucketColl *mongo.Collection,
) bool {
	ctx := s.Context()
	cursor, err := bucketColl.Database().RunCommandCursor(ctx, bson.D{
		{"aggregate", bucketColl.Name()},
		{"pipeline", bson.A{
			bson.D{{"$listCatalog", bson.D{}}},
		}},
		{"readConcern", bson.D{{"level", "majority"}}},
		{"cursor", bson.D{}},
	})
	s.Require().NoError(err)

	if !cursor.Next(ctx) {
		s.Require().Fail("no entry in $listCatalog response")
	}

	md, err := cursor.Current.LookupErr("md")
	s.Require().NoError(err, "lookup 'md' field")

	hasMixedSchema, err := md.Document().LookupErr("timeseriesBucketsMayHaveMixedSchemaData")
	s.Require().NoError(err, "lookup 'timeseriesBucketsMayHaveMixedSchemaData' field")

	return hasMixedSchema.Boolean()
}

func (s *DumpRestoreSuite) setupTimeseriesWithMixedSchema(dbName string, collName string) {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "get session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	s.Require().NoError(err, "get server version")

	client, err := sessionProvider.GetSession()
	s.Require().NoError(err, "get session")

	err = client.Database(dbName).Collection(collName).Drop(s.Context())
	s.Require().NoError(err, "drop existing coll")

	createCmd := bson.D{
		{"create", collName},
		{"timeseries", bson.D{
			{"timeField", "t"},
			{"metaField", "m"},
		}},
	}

	createRes := sessionProvider.DB(dbName).RunCommand(s.Context(), createCmd)
	s.Require().NoError(createRes.Err(), "create timeseries coll")

	// SERVER-84531 was only backported to 7.3.
	if cmp, err := testutil.CompareFCV(testutil.GetFCV(client), "7.3"); err != nil || cmp >= 0 {
		res := sessionProvider.DB(dbName).RunCommand(s.Context(), bson.D{
			{"collMod", collName},
			{"timeseriesBucketsMayHaveMixedSchemaData", true},
		})

		s.Require().NoError(res.Err(), "collMod timeseries collection")
	}

	bucketName := timeseriesCollName(serverVersion, collName)
	bucketColl := sessionProvider.DB(dbName).Collection(bucketName)
	bucketJSON := `{"_id":{"$oid":"65a6eb806ffc9fa4280ecac4"},"control":{"version":1,"min":{"_id":{"$oid":"65a6eba7e6d2e848e08c3750"},"t":{"$date":"2024-01-16T20:48:00Z"},"a":1},"max":{"_id":{"$oid":"65a6eba7e6d2e848e08c3751"},"t":{"$date":"2024-01-16T20:48:39.448Z"},"a":"a"}},"meta":0,"data":{"_id":{"0":{"$oid":"65a6eba7e6d2e848e08c3750"},"1":{"$oid":"65a6eba7e6d2e848e08c3751"}},"t":{"0":{"$date":"2024-01-16T20:48:39.448Z"},"1":{"$date":"2024-01-16T20:48:39.448Z"}},"a":{"0":"a","1":1}}}`
	var bucketMap map[string]any
	err = json.Unmarshal([]byte(bucketJSON), &bucketMap)
	s.Require().NoError(err, "unmarshal json")

	err = bsonutil.ConvertLegacyExtJSONDocumentToBSON(bucketMap)
	s.Require().NoError(err, "convert extjson to bson")

	_, err = bucketColl.InsertOne(s.Context(), bucketMap)
	s.Require().NoError(err, "insert bucket doc")
}
