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
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/integration/sharedsuite"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

type DumpRestoreSuite struct {
	sharedsuite.IntegrationSuite
}

func TestDumpRestore(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	suite.Run(t, new(DumpRestoreSuite))
}

func (s *DumpRestoreSuite) withBSONMongodump(testCase func(string), args ...string) {
	s.withBSONMongodumpForURI(os.Getenv(testopts.URIEnvVar), testCase, args...)
}

func (s *DumpRestoreSuite) withBSONMongodumpForURI(
	uri string,
	testCase func(string),
	args ...string,
) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	dirArgs := []string{
		"--out", dir,
	}
	s.runMongodumpWithArgsForURI(uri, append(dirArgs, args...)...)
	testCase(dir)
}

func (s *DumpRestoreSuite) runMongodumpWithArgs(args ...string) {
	s.runMongodumpWithArgsForURI(os.Getenv(testopts.URIEnvVar), args...)
}

func (s *DumpRestoreSuite) runMongodumpWithArgsForURI(uri string, args ...string) {
	cmd := []string{"go", "run", filepath.Join("..", "..", "mongodump", "main")}
	cmd = append(cmd, testopts.GetBareArgsForURI(uri)...)
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
	s.withBSONMongodumpForCollectionForURI(os.Getenv(testopts.URIEnvVar), db, collection, testCase)
}

func (s *DumpRestoreSuite) withBSONMongodumpForCollectionForURI(
	uri, db, collection string,
	testCase func(string),
) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	s.runBSONMongodumpForCollectionForURI(uri, dir, db, collection)
	testCase(dir)
}

func (s *DumpRestoreSuite) runBSONMongodumpForCollectionForURI(
	uri, dir, db, collection string,
	args ...string,
) string {
	baseArgs := []string{
		"--out", dir,
		"--db", db,
		"--collection", collection,
	}
	s.runMongodumpWithArgsForURI(
		uri,
		append(baseArgs, args...)...,
	)
	bsonFile := filepath.Join(dir, db, fmt.Sprintf("%s.bson", collection))
	_, err := os.Stat(bsonFile)
	s.Require().NoError(err, "dump created BSON data file")
	_, err = os.Stat(filepath.Join(dir, db, fmt.Sprintf("%s.metadata.json", collection)))
	s.Require().NoError(err, "dump created JSON metadata file")
	return bsonFile
}

func (s *DumpRestoreSuite) withArchiveMongodumpForURI(
	uri string,
	testCase func(string),
	dumpArgs ...string,
) {
	dir, cleanup := testutil.MakeTempDir(s.T())
	defer cleanup()
	file := filepath.Join(dir, "archive")
	s.runArchiveMongodumpForURI(uri, file, dumpArgs...)
	testCase(file)
}

func (s *DumpRestoreSuite) runArchiveMongodumpForURI(uri, file string, dumpArgs ...string) {
	s.runMongodumpWithArgsForURI(
		uri,
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
	sessionProvider, _, err := testutil.GetBareSessionProvider(s.T())
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

// database returns a handle to a database named for the test that asked for it.
// Each test names its own database rather than deriving one from the test name,
// because the suite's DBName truncates to 63 bytes, which leaves too few
// distinguishing characters for these deeply nested subtests.
func (s *DumpRestoreSuite) database(name string) *mongo.Database {
	session, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to the server")

	return session.Database("dumprestore_" + name)
}

// crossCluster names the two clusters a round-trip test runs between and which
// side each role maps to. Setup and dump use the source; restore and the
// post-restore assertions use the target.
type crossCluster struct {
	source, target       *mongo.Client
	sourceURI, targetURI string
}

// withOrientations runs a round-trip test body once per source/target
// orientation. In single-cluster mode it runs once, with source and target both
// the primary cluster. In cross-cluster mode it runs twice, once with each
// cluster as the source, so a test exercises the boundary in both directions.
func (s *DumpRestoreSuite) withOrientations(body func(crossCluster)) {
	primaryURI := os.Getenv(testopts.URIEnvVar)
	secondURI := testopts.SecondURI()

	if secondURI == "" {
		session, err := testutil.GetBareSession(s.T())
		s.Require().NoError(err, "can connect to the server")
		body(crossCluster{
			source:    session,
			target:    session,
			sourceURI: primaryURI,
			targetURI: primaryURI,
		})
		return
	}

	for _, orientation := range []struct {
		source, target string
	}{
		{primaryURI, secondURI},
		{secondURI, primaryURI},
	} {
		s.Run(
			fmt.Sprintf(
				"dump from %s restore into %s",
				uriLabel(orientation.source),
				uriLabel(orientation.target),
			),
			func() {
				source, err := testutil.GetBareSessionForURI(s.T(), orientation.source)
				s.Require().NoError(err, "can connect to the source cluster")
				target, err := testutil.GetBareSessionForURI(s.T(), orientation.target)
				s.Require().NoError(err, "can connect to the target cluster")

				body(crossCluster{
					source:    source,
					target:    target,
					sourceURI: orientation.source,
					targetURI: orientation.target,
				})
			},
		)
	}
}

// uriLabel returns a short human-readable name for a cluster URI, for use in
// test subtest names.
func uriLabel(uri string) string {
	cs, err := connstring.ParseAndValidate(uri)
	if err != nil {
		return uri
	}
	return strings.Join(cs.Hosts, ",")
}

func (s *DumpRestoreSuite) createCollection(
	testDB *mongo.Database,
	collName string,
	opts *options.CreateCollectionOptionsBuilder,
) *mongo.Collection {
	err := testDB.CreateCollection(s.Context(), collName, opts)
	s.Require().NoError(err, "can create the collection %#q", collName)

	return testDB.Collection(collName)
}

// newDumpDir creates a dump directory holding one database directory, for tests
// that need a dump mongodump would not produce. It returns the dump root and the
// database directory inside it.
func (s *DumpRestoreSuite) newDumpDir(dbName string) (string, string) {
	root := s.T().TempDir()
	dbDir := filepath.Join(root, dbName)
	s.Require().NoError(os.MkdirAll(dbDir, 0755), "can create the dump directory")

	return root, dbDir
}

// writeBSONFile writes documents in the concatenated-BSON format that mongodump
// produces for a collection.
func (s *DumpRestoreSuite) writeBSONFile(path string, docs ...bson.D) {
	var buf []byte
	for _, doc := range docs {
		marshaled, err := bson.Marshal(doc)
		s.Require().NoError(err, "can marshal a document into the bson file")
		buf = append(buf, marshaled...)
	}

	s.Require().NoError(os.WriteFile(path, buf, 0644), "can write the bson file %#q", path)
}

func (s *DumpRestoreSuite) runRestore(args ...string) mongorestore.Result {
	restore, err := getRestoreWithArgs(args...)
	s.Require().NoError(err, "can build mongorestore")
	defer restore.Close()

	return restore.Restore()
}

// dropDB drops the database and verifies that it is empty, so that a restore
// which silently does nothing cannot pass by leaving the original data in place.
func (s *DumpRestoreSuite) dropDB(testDB *mongo.Database) {
	s.Require().NoError(
		testDB.Drop(s.Context()),
		"can drop the database %#q",
		testDB.Name(),
	)
	s.Require().Empty(
		s.collectionNames(testDB),
		"dropping %#q removes all of its collections",
		testDB.Name(),
	)
}

func (s *DumpRestoreSuite) dropCollection(coll *mongo.Collection) {
	s.Require().NoError(coll.Drop(s.Context()), "can drop the collection %#q", coll.Name())
	s.Require().NotContains(
		s.collectionNames(coll.Database()),
		coll.Name(),
		"dropping %#q removes it",
		coll.Name(),
	)
}

func (s *DumpRestoreSuite) collectionNames(testDB *mongo.Database) []string {
	names, err := testDB.ListCollectionNames(s.Context(), bson.D{})
	s.Require().NoError(err, "can list the collections in %#q", testDB.Name())

	return names
}

func (s *DumpRestoreSuite) docCount(coll *mongo.Collection) int64 {
	count, err := coll.CountDocuments(s.Context(), bson.D{})
	s.Require().NoError(err, "can count documents in %#q", coll.Name())

	return count
}

func (s *DumpRestoreSuite) indexNames(coll *mongo.Collection) []string {
	specs, err := coll.Indexes().ListSpecifications(s.Context())
	s.Require().NoError(err, "can list indexes on %#q", coll.Name())

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}

	return names
}

// indexKey returns one index's key document as a bson.D, so that the value
// types and field order are visible to assertions.
func (s *DumpRestoreSuite) indexKey(coll *mongo.Collection, indexName string) bson.D {
	cursor, err := coll.Indexes().List(s.Context())
	s.Require().NoError(err, "can list indexes on %#q", coll.Name())

	var indexes []struct {
		Name string `bson:"name"`
		Key  bson.D `bson:"key"`
	}
	s.Require().NoError(cursor.All(s.Context(), &indexes), "can read the index specs")

	for _, index := range indexes {
		if index.Name == indexName {
			return index.Key
		}
	}

	s.Require().Failf(
		"index not found",
		"the index %#q exists on %#q",
		indexName,
		coll.Name(),
	)

	return nil
}

// collectionOptions returns the collection's creation options, which is where
// the server reports capped settings, validators, and collations.
func (s *DumpRestoreSuite) collectionOptions(testDB *mongo.Database, collName string) bson.D {
	cursor, err := testDB.ListCollections(s.Context(), bson.D{{"name", collName}})
	s.Require().NoError(err, "can list collections")

	var infos []struct {
		Options bson.D `bson:"options"`
	}
	s.Require().NoError(cursor.All(s.Context(), &infos), "can read the collection info")
	s.Require().Len(infos, 1, "the collection %#q exists", collName)

	return infos[0].Options
}

// collectionOption returns one field of a collection's options.
func (s *DumpRestoreSuite) collectionOption(
	testDB *mongo.Database,
	collName string,
	key string,
) any {
	return optionValue(s.collectionOptions(testDB, collName), key)
}

// optionValue returns one field of an options document, or nil when the server
// does not report that field at all.
func optionValue(options bson.D, key string) any {
	value, err := bsonutil.FindValueByKey(key, &options)
	if err != nil {
		return nil
	}

	return value
}
