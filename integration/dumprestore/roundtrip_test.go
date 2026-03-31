package dumprestore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/sync/errgroup"
)

func uniqueDBName() string {
	return fmt.Sprintf("mongorestore_test_%d_%d", os.Getpid(), time.Now().UnixMilli())
}

func (s *DumpRestoreSuite) TestPipedDumpRestore() {
	s.T().Logf("start %#q", s.T().Name())
	ctx := s.T().Context()

	provider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "should get session provider")

	s.T().Logf("getting session")
	sess, err := provider.GetSession()
	s.Require().NoError(err, "should get session")

	srcCollNames := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	db := sess.Database(uniqueDBName())
	s.Require().NoError(db.Drop(ctx), "should pre-drop DB %#q", db.Name())

	s.T().Logf("creating collections")

	for _, collName := range srcCollNames {
		docs := lo.RepeatBy(
			10_000,
			func(_ int) bson.D {
				return bson.D{
					{"someNum", rand.Float64()},
				}
			},
		)

		s.Require().NoError(
			db.Collection(collName).Drop(ctx),
			"should drop %#q", collName,
		)

		_, err := db.Collection(collName).InsertMany(
			ctx,
			lo.ToAnySlice(docs),
		)

		s.Require().NoError(err, "should insert docs into %#q", collName)
	}

	s.T().Log("Finished creating documents.")

	reader, writer := io.Pipe()

	eg, _ := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer writer.Close()

		dump, err := getArchiveMongoDump(writer)
		if err != nil {
			return errors.Wrap(err, "create mongodump")
		}

		dump.ToolOptions.DB = db.Name()

		assert.NoError(s.T(), dump.Dump(), "dump should work")

		return nil
	})

	eg.Go(func() error {
		defer reader.Close()

		restore, err := getArchiveMongoRestore(reader)
		if err != nil {
			return errors.Wrap(err, "create mongorestore")
		}

		restore.NSOptions = &mongorestore.NSOptions{
			NSFrom: lo.Map(
				srcCollNames,
				func(cn string, _ int) string {
					return db.Name() + "." + cn
				},
			),
			NSTo: lo.Map(
				srcCollNames,
				func(cn string, _ int) string {
					return db.Name() + ".dst-" + cn
				},
			),
		}

		assert.NoError(s.T(), restore.Restore().Err, "restore should work")

		return nil
	})

	s.Require().NoError(eg.Wait())
}

func (s *DumpRestoreSuite) TestDumpAndRestoreConfigDB() {
	_, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to server")

	s.Run(
		"test dump and restore only config db includes all config collections",
		func() {
			testDumpAndRestoreConfigDBIncludesAllCollections(s.T())
		},
	)

	s.Run(
		"test dump and restore all dbs includes only some config collections",
		func() {
			testDumpAndRestoreAllDBsIgnoresSomeConfigCollections(s.T())
		},
	)
}

var testDocument = bson.M{"key": "value"}

var configCollectionNamesToKeep = []string{
	"chunks",
	"collections",
	"databases",
	"settings",
	"shards",
	"tags",
	"version",
}

var userDefinedConfigCollectionNames = []string{
	"coll1",
	"coll2",
	"coll3",
}

func testDumpAndRestoreConfigDBIncludesAllCollections(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	configDB := session.Database("config")

	collections := createCollectionsWithTestDocuments(
		t,
		configDB,
		append(configCollectionNamesToKeep, userDefinedConfigCollectionNames...),
	)
	defer clearDB(t, configDB)

	withBSONMongodump(
		t,
		func(dir string) {
			clearDB(t, configDB)

			restore, err := getRestoreWithArgs(dir)
			require.NoError(err)
			defer restore.Close()

			result := restore.Restore()
			require.NoError(result.Err, "can run mongorestore")
			require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

			for _, collection := range collections {
				r := collection.FindOne(t.Context(), testDocument)
				require.NoError(r.Err(), "expected document")
			}
		},
		"--db", "config",
		"--excludeCollection", "transactions",
	)
}

func testDumpAndRestoreAllDBsIgnoresSomeConfigCollections(t *testing.T) {
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	// Drop any databases that other tests may have left behind with validators
	// that would cause failures during the full dump+restore.
	require.NoError(session.Database("mongodump_test_db").Drop(t.Context()))

	configDB := session.Database("config")

	userDefinedCollections := createCollectionsWithTestDocuments(
		t,
		configDB,
		userDefinedConfigCollectionNames,
	)
	collectionsToKeep := createCollectionsWithTestDocuments(
		t,
		configDB,
		configCollectionNamesToKeep,
	)
	defer clearDB(t, configDB)

	withBSONMongodump(
		t,
		func(dir string) {
			clearDB(t, configDB)

			restore, err := getRestoreWithArgs(
				mongorestore.DropOption,
				dir,
			)
			require.NoError(err)
			defer restore.Close()

			result := restore.Restore()
			require.NoError(result.Err, "can run mongorestore")
			require.EqualValues(0, result.Failures, "mongorestore reports 0 failures")

			for _, collection := range collectionsToKeep {
				r := collection.FindOne(t.Context(), testDocument)
				require.NoError(r.Err(), "expected document")
			}

			for _, collection := range userDefinedCollections {
				r := collection.FindOne(t.Context(), testDocument)
				require.Error(r.Err(), "expected no document")
			}
		},
	)
}

func withBSONMongodump(t *testing.T, testCase func(string), args ...string) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	dirArgs := []string{
		"--out", dir,
	}
	runMongodumpWithArgs(t, append(dirArgs, args...)...)
	testCase(dir)
}

func runMongodumpWithArgs(t *testing.T, args ...string) {
	require := require.New(t)
	cmd := []string{"go", "run", filepath.Join("..", "..", "mongodump", "main")}
	cmd = append(cmd, testutil.GetBareArgs()...)
	cmd = append(cmd, args...)
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	cmdStr := strings.Join(cmd, " ")
	require.NoError(err, "can execute command %s with output: %s", cmdStr, out)
	require.NotContains(
		string(out),
		"does not exist",
		"running [%s] does not tell us the namespace does not exist",
		cmdStr,
	)

	// So we can see dump's output when debugging test failures:
	fmt.Print(string(out))
}

func getRestoreWithArgs(additionalArgs ...string) (*mongorestore.MongoRestore, error) {
	opts, err := mongorestore.ParseOptions(
		append(testutil.GetBareArgs(), additionalArgs...),
		"",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("error parsing args: %v", err)
	}

	restore, err := mongorestore.New(opts)
	if err != nil {
		return nil, fmt.Errorf("error making new instance of mongorestore: %v", err)
	}

	return restore, nil
}

func getArchiveMongoDump(output io.WriteCloser) (*mongodump.MongoDump, error) {
	provider, toolOpts, err := testutil.GetBareSessionProvider()
	if err != nil {
		return nil, errors.Wrap(err, "get session provider for dump")
	}

	dump := &mongodump.MongoDump{
		InputOptions: &mongodump.InputOptions{},
		OutputOptions: &mongodump.OutputOptions{
			Archive:                "-",
			NumParallelCollections: 4, // default
		},
		SessionProvider: provider,
		ToolOptions:     toolOpts,
		OutputWriter:    output,
	}

	err = dump.Init()
	if err != nil {
		return nil, errors.Wrap(err, "init mongodump")
	}

	return dump, nil
}

func getArchiveMongoRestore(input io.ReadCloser) (*mongorestore.MongoRestore, error) {
	_, toolOpts, err := testutil.GetBareSessionProvider()
	if err != nil {
		return nil, errors.Wrap(err, "get session provider for restore")
	}

	restore, err := mongorestore.New(mongorestore.Options{
		ToolOptions: toolOpts,
		InputOptions: &mongorestore.InputOptions{
			Archive: "-",
		},
		OutputOptions: &mongorestore.OutputOptions{
			NumInsertionWorkers: 1,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "create mongorestore")
	}
	restore.InputReader = input

	return restore, nil
}

func (s *DumpRestoreSuite) TestRestoreUsersOrRoles() {
	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "no server available")

	s.Run("Restoring users and roles should drop tempusers and temproles", func() {
		restore, err := getRestoreWithArgs(
			mongorestore.NumParallelCollectionsOption, "1",
			mongorestore.NumInsertionWorkersOption, "1",
		)
		s.Require().NoError(err)
		defer restore.Close()

		adminDB := session.Database("admin")
		restore.TargetDirectory = "../../mongorestore/testdata/usersdump"
		result := restore.Restore()
		s.Require().NoError(result.Err, "can run mongorestore")

		adminCollections, err := adminDB.ListCollectionNames(s.T().Context(), bson.M{})
		s.Require().NoError(err, "can list admin collections")

		for _, collName := range adminCollections {
			s.NotEqual("tempusers", collName, "tempusers should not exist after restore")
			s.NotEqual("temproles", collName, "temproles should not exist after restore")
		}
	})

	s.Run("If --dumpUsersAndRoles was not used with the target", func() {
		s.Run("Restoring from db directory should not be allowed", func() {
			restore, err := getRestoreWithArgs(
				mongorestore.NumParallelCollectionsOption, "1",
				mongorestore.NumInsertionWorkersOption, "1",
				mongorestore.RestoreDBUsersAndRolesOption,
				mongorestore.DBOption,
				"db1",
				"../../mongorestore/testdata/testdirs/db1",
			)
			s.Require().NoError(err)
			defer restore.Close()

			result := restore.Restore()
			s.Require().
				ErrorIs(result.Err, mongorestore.NoUsersOrRolesInDumpError, "should get NoUsersOrRolesInDumpError")
		})

		s.Run("Restoring from base dump directory should not be allowed", func() {
			restore, err := getRestoreWithArgs(
				mongorestore.NumParallelCollectionsOption, "1",
				mongorestore.NumInsertionWorkersOption, "1",
				mongorestore.RestoreDBUsersAndRolesOption,
				mongorestore.DBOption,
				"db1",
				"../../mongorestore/testdata/testdirs",
			)
			s.Require().NoError(err)
			defer restore.Close()

			result := restore.Restore()
			s.Require().
				ErrorIs(result.Err, mongorestore.NoUsersOrRolesInDumpError, "should get NoUsersOrRolesInDumpError")
		})

		s.Run("Restoring from archive of entire dump should not be allowed", func() {
			withArchiveMongodump(s.T(), func(archivePath string) {
				restore, err := getRestoreWithArgs(
					mongorestore.NumParallelCollectionsOption, "1",
					mongorestore.NumInsertionWorkersOption, "1",
					mongorestore.RestoreDBUsersAndRolesOption,
					mongorestore.DBOption,
					"db1",
					mongorestore.ArchiveOption+"="+archivePath,
				)
				s.Require().NoError(err)
				defer restore.Close()

				result := restore.Restore()
				s.Require().
					ErrorIs(result.Err, mongorestore.NoUsersOrRolesInDumpError, "should get NoUsersOrRolesInDumpError")
			})
		})
	})
}

func (s *DumpRestoreSuite) TestUnversionedIndexes() {
	ctx := s.T().Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	s.Require().NoError(err, "no client available")

	serverVersion, err := sessionProvider.ServerVersionArray()
	s.Require().NoError(err, "get cluster version")

	dbName := s.T().Name()
	collName := "coll"

	coll := session.Database(dbName).Collection(collName)

	metadataEJSON, err := bson.MarshalExtJSON(
		bson.D{
			{"collectionName", collName},
			{"type", "collection"},
			{"uuid", uuid.New().String()},
			{"indexes", []bson.D{
				{
					{"v", 2},
					{"key", bson.D{{"_id", 1}}},
					{"name", "_id_"},
				},
				{
					{"v", 2},
					{"key", bson.D{{"myfield", "2dsphere"}}},
					{"name", "my2dsphere"},
				},
			}},
		},
		false,
		false,
	)
	s.Require().NoError(err, "should marshal metadata to extJSON")

	simpleArchive := archive.SimpleArchive{
		Header: archive.Header{
			ServerVersion: serverVersion.String(),
		},
		CollectionMetadata: []archive.CollectionMetadata{
			{
				Database:   dbName,
				Collection: collName,
				Metadata:   string(metadataEJSON),
				Size:       0,
			},
		},
		Namespaces: []archive.SimpleNamespace{
			{
				Database:   dbName,
				Collection: collName,
			},
		},
	}
	archiveBytes, err := simpleArchive.Marshal()
	s.Require().NoError(err, "should marshal the archive")

	withArchiveMongodump(s.T(), func(archivePath string) {
		s.Require().NoError(os.WriteFile(archivePath, archiveBytes, 0644))

		restore, err := getRestoreWithArgs(
			mongorestore.DropOption,
			mongorestore.ArchiveOption+"="+archivePath,
		)
		s.Require().NoError(err)
		defer restore.Close()

		result := restore.Restore()
		s.Require().NoError(result.Err, "can run mongorestore")
		s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		cursor, err := coll.Indexes().List(ctx)
		s.Require().NoError(err, "should open index-list cursor")

		var indexes []idx.IndexDocument
		err = cursor.All(ctx, &indexes)
		s.Require().NoError(err, "should fetch index specs")

		s.T().Logf("indexes: %+v", indexes)

		var twoDIndexDoc idx.IndexDocument
		for _, index := range indexes {
			if index.Options["name"] == "my2dsphere" {
				twoDIndexDoc = index
			}
		}

		s.Require().NotNil(twoDIndexDoc.Key, "should find 2dsphere index (indexes: %+v)", indexes)
		s.Equal(
			int32(1),
			twoDIndexDoc.Options["2dsphereIndexVersion"],
			"should have version 1 2dsphere index (unversioned)",
		)
	})
}

func (s *DumpRestoreSuite) TestRestoreTimeseriesCollectionsWithMixedSchema() {
	ctx := s.T().Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err, "no cluster available")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	s.Require().NoError(err, "no client available")

	fcv := testutil.GetFCV(session)
	// TODO: Enable tests for 6.0, 7.0 and 8.0 (TOOLS-3597).
	// The server fix for SERVER-84531 was only backported to 7.3.
	if cmp, err := testutil.CompareFCV(fcv, "7.3"); err != nil || cmp < 0 {
		s.Require().NoError(err, "get fcv")
		s.T().Skip("Requires server with FCV 7.3 or later")
	}

	if cmp, err := testutil.CompareFCV(fcv, "8.0"); cmp >= 0 {
		s.Require().NoError(err, "get fcv")
		s.T().Skip("The test currently fails on v8.0 because of SERVER-92222")
	}

	serverVersion, err := sessionProvider.ServerVersionArray()
	s.Require().NoError(err, "parse server version")

	dbName := "timeseries_test_DB"
	collName := "timeseries_mixed_schema"
	testdb := session.Database(dbName)
	bucketColl := testdb.Collection(timeseriesCollName(serverVersion, collName))

	setupTimeseriesWithMixedSchema(s.T(), dbName, collName)

	withArchiveMongodump(s.T(), func(file string) {
		s.Require().NoError(testdb.Collection(collName).Drop(ctx))
		s.Require().NoError(bucketColl.Drop(ctx))

		restore, err := getRestoreWithArgs(
			mongorestore.DropOption,
			mongorestore.ArchiveOption+"="+file,
		)
		s.Require().NoError(err)
		defer restore.Close()

		result := restore.Restore()
		s.Require().NoError(result.Err, "can run mongorestore")
		s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

		count, err := testdb.Collection(collName).CountDocuments(ctx, bson.M{})
		s.Require().NoError(err)
		s.Require().Equal(int64(2), count, "should have 2 documents in timeseries collection")

		count, err = bucketColl.CountDocuments(ctx, bson.M{})
		s.Require().NoError(err)
		s.Require().Equal(int64(1), count, "should have 1 document in bucket collection")

		hasMixedSchema := timeseriesBucketsMayHaveMixedSchemaData(s.T(), bucketColl)
		s.Require().True(hasMixedSchema, "bucket collection should have mixed schema flag set")

		//nolint:errcheck
		defer testdb.Collection(collName).Drop(ctx)
	})
}

func (s *DumpRestoreSuite) TestIgnoreMongoDBInternal() {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	s.Require().NoError(err)

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		s.T().Skip("replica set required")
	}

	ctx := s.T().Context()

	dbName := util.MongoDBInternalDBPrefix + s.T().Name()

	client, err := testutil.GetBareSession()
	s.Require().NoError(err, "must connect to server")

	internalColl := client.Database(dbName).Collection(s.T().Name())

	_, err = internalColl.InsertOne(ctx, bson.D{{"_id", 1}})
	s.Require().NoError(err, "must write to the internal DB")

	_, err = client.Database(s.T().Name()).Collection(s.T().Name()).InsertOne(ctx, bson.D{})
	s.Require().NoError(err, "must write to the user DB")

	writesCtx, writesCancel := context.WithCancelCause(ctx)
	updatesDone := make(chan struct{})
	go func() {
		defer close(updatesDone)

		for writesCtx.Err() == nil {
			_, err := internalColl.InsertOne(
				writesCtx,
				bson.D{},
			)

			if !errors.Is(err, context.Canceled) {
				s.Require().NoError(err, "must write to the internal DB")
			}
		}

		s.T().Logf("Updates canceled: %v", context.Cause(writesCtx))
	}()

	withArchiveMongodump(
		s.T(),
		func(archivePath string) {
			writesCancel(fmt.Errorf("archive is finished"))
			<-updatesDone

			s.Require().NoError(client.Database(internalColl.Database().Name()).Drop(ctx))
			s.Require().NoError(client.Database(s.T().Name()).Drop(ctx))

			restore, err := getRestoreWithArgs(
				mongorestore.ArchiveOption+"="+archivePath,
				"-vv",
				"--oplogReplay",
				"--drop",
			)
			s.Require().NoError(err)
			defer restore.Close()

			result := restore.Restore()
			s.Require().NoError(result.Err, "can run mongorestore")
			s.Require().EqualValues(
				0,
				result.Failures,
				"mongorestore reports 0 failures (result=%+v)",
				result,
			)
		},
		"--oplog",
		"-vv",
	)

	dbNames, err := client.ListDatabaseNames(ctx, bson.D{})
	s.Require().NoError(err)

	s.Contains(dbNames, s.T().Name(), "user DB restored")
	s.NotContains(dbNames, internalColl.Database().Name(), "internal DB ignored")
}

func (s *DumpRestoreSuite) TestFinalNewlinesInNamespaces() {
	ctx := s.T().Context()

	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to server")

	allNames := []string{
		"no-nl",
		"\ninitial-nl",
		"mid-\n-nl",
		"final-nl\n",
		"\ninitial-and-final-nl\n",
		"\nnl-\n-everywhere\n",
	}

	nlVariants := []struct {
		label string
		nl    string
	}{
		{"LF", "\n"},
		{"CR", "\r"},
		{"CRLF", "\r\n"},
	}

	for _, variant := range nlVariants {
		myAllNames := lo.Map(
			allNames,
			func(name string, _ int) string {
				return strings.ReplaceAll(name, "\n", variant.nl)
			},
		)

		s.Run(
			variant.label,
			func() {
				for _, dbname := range myAllNames {
					s.Run(
						fmt.Sprintf("dbname=%s", strconv.Quote(dbname)),
						func() {
							s.Require().NoError(session.Database(dbname).Drop(ctx))
							createCollectionsWithTestDocuments(
								s.T(),
								session.Database(dbname),
								myAllNames,
							)

							withArchiveMongodump(s.T(), func(archivePath string) {
								s.Require().NoError(session.Database(dbname).Drop(ctx))

								colls, err := session.Database(dbname).
									ListCollectionNames(ctx, bson.D{})
								s.Require().NoError(err)
								s.Require().
									Empty(colls, "sanity: db drop should drop all collections")

								restore, err := getRestoreWithArgs(
									mongorestore.DBOption, dbname,
									mongorestore.ArchiveOption+"="+archivePath,
									"-vv",
								)
								s.Require().NoError(err)
								defer restore.Close()

								result := restore.Restore()
								s.Require().NoError(result.Err, "can run mongorestore")
								s.Require().EqualValues(
									0,
									result.Failures,
									"mongorestore reports 0 failures (result=%+v)",
									result,
								)
							})

							colls, err := session.Database(dbname).
								ListCollectionNames(ctx, bson.D{})
							s.Require().NoError(err)

							assert.ElementsMatch(
								s.T(),
								myAllNames,
								colls,
								"all collections restored",
							)
						},
					)
				}
			},
		)
	}
}

func withArchiveMongodump(t *testing.T, testCase func(string), dumpArgs ...string) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	file := filepath.Join(dir, "archive")
	runArchiveMongodump(t, file, dumpArgs...)
	testCase(file)
}

func runArchiveMongodump(t *testing.T, file string, dumpArgs ...string) {
	runMongodumpWithArgs(
		t,
		append(
			[]string{mongorestore.ArchiveOption + "=" + file},
			dumpArgs...,
		)...,
	)
	_, err := os.Stat(file)
	require.NoError(t, err, "dump created archive data file")
}

func timeseriesBucketsMayHaveMixedSchemaData(
	t *testing.T,
	bucketColl *mongo.Collection,
) bool {
	ctx := t.Context()
	cursor, err := bucketColl.Database().RunCommandCursor(ctx, bson.D{
		{"aggregate", bucketColl.Name()},
		{"pipeline", bson.A{
			bson.D{{"$listCatalog", bson.D{}}},
		}},
		{"readConcern", bson.D{{"level", "majority"}}},
		{"cursor", bson.D{}},
	})
	require.NoError(t, err)

	if !cursor.Next(ctx) {
		require.Fail(t, "no entry in $listCatalog response")
	}

	md, err := cursor.Current.LookupErr("md")
	require.NoError(t, err, "lookup 'md' field")

	hasMixedSchema, err := md.Document().LookupErr("timeseriesBucketsMayHaveMixedSchemaData")
	require.NoError(t, err, "lookup 'timeseriesBucketsMayHaveMixedSchemaData' field")

	return hasMixedSchema.Boolean()
}

func setupTimeseriesWithMixedSchema(t *testing.T, dbName string, collName string) {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "get server version")

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "get session")

	err = client.Database(dbName).Collection(collName).Drop(t.Context())
	require.NoError(t, err, "drop existing coll")

	createCmd := bson.D{
		{"create", collName},
		{"timeseries", bson.D{
			{"timeField", "t"},
			{"metaField", "m"},
		}},
	}

	createRes := sessionProvider.DB(dbName).RunCommand(t.Context(), createCmd)
	require.NoError(t, createRes.Err(), "create timeseries coll")

	// SERVER-84531 was only backported to 7.3.
	if cmp, err := testutil.CompareFCV(testutil.GetFCV(client), "7.3"); err != nil || cmp >= 0 {
		res := sessionProvider.DB(dbName).RunCommand(t.Context(), bson.D{
			{"collMod", collName},
			{"timeseriesBucketsMayHaveMixedSchemaData", true},
		})

		require.NoError(t, res.Err(), "collMod timeseries collection")
	}

	bucketName := timeseriesCollName(serverVersion, collName)
	bucketColl := sessionProvider.DB(dbName).Collection(bucketName)
	bucketJSON := `{"_id":{"$oid":"65a6eb806ffc9fa4280ecac4"},"control":{"version":1,"min":{"_id":{"$oid":"65a6eba7e6d2e848e08c3750"},"t":{"$date":"2024-01-16T20:48:00Z"},"a":1},"max":{"_id":{"$oid":"65a6eba7e6d2e848e08c3751"},"t":{"$date":"2024-01-16T20:48:39.448Z"},"a":"a"}},"meta":0,"data":{"_id":{"0":{"$oid":"65a6eba7e6d2e848e08c3750"},"1":{"$oid":"65a6eba7e6d2e848e08c3751"}},"t":{"0":{"$date":"2024-01-16T20:48:39.448Z"},"1":{"$date":"2024-01-16T20:48:39.448Z"}},"a":{"0":"a","1":1}}}`
	var bucketMap map[string]any
	err = json.Unmarshal([]byte(bucketJSON), &bucketMap)
	require.NoError(t, err, "unmarshal json")

	err = bsonutil.ConvertLegacyExtJSONDocumentToBSON(bucketMap)
	require.NoError(t, err, "convert extjson to bson")

	_, err = bucketColl.InsertOne(t.Context(), bucketMap)
	require.NoError(t, err, "insert bucket doc")
}

func timeseriesCollName(version db.Version, base string) string {
	if version.SupportsRawData() {
		return base
	}

	return common.TimeseriesBucketPrefix + base
}

func createCollectionsWithTestDocuments(
	t *testing.T,
	db *mongo.Database,
	collectionNames []string,
) []*mongo.Collection {
	collections := []*mongo.Collection{}
	for _, collectionName := range collectionNames {
		collection := createCollectionWithTestDocument(t, db, collectionName)
		collections = append(collections, collection)
	}
	return collections
}

func createCollectionWithTestDocument(
	t *testing.T,
	db *mongo.Database,
	collectionName string,
) *mongo.Collection {
	require := require.New(t)
	collection := db.Collection(collectionName)
	_, err := collection.InsertOne(
		t.Context(),
		testDocument,
	)
	require.NoError(err, "can insert documents into collection")
	return collection
}

func clearDB(t *testing.T, db *mongo.Database) {
	require := require.New(t)
	collectionNames, err := db.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(err, "can get collection names")
	for _, collectionName := range collectionNames {
		collection := db.Collection(collectionName)
		_, _ = collection.DeleteMany(t.Context(), bson.M{})
	}
}

func withBSONMongodumpForCollection(
	t *testing.T,
	db string,
	collection string,
	testCase func(string),
) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	runBSONMongodumpForCollection(t, dir, db, collection)
	testCase(dir)
}

func runBSONMongodumpForCollection(
	t *testing.T,
	dir, db, collection string,
	args ...string,
) string {
	require := require.New(t)
	baseArgs := []string{
		"--out", dir,
		"--db", db,
		"--collection", collection,
	}
	runMongodumpWithArgs(
		t,
		append(baseArgs, args...)...,
	)
	bsonFile := filepath.Join(dir, db, fmt.Sprintf("%s.bson", collection))
	_, err := os.Stat(bsonFile)
	require.NoError(err, "dump created BSON data file")
	_, err = os.Stat(filepath.Join(dir, db, fmt.Sprintf("%s.metadata.json", collection)))
	require.NoError(err, "dump created JSON metadata file")
	return bsonFile
}

// listIndexes lists all indexes on a collection and decodes them into target.
// ListSpecifications returns IndexSpecifications, which don't describe all
// parts of the index, so we List() directly to capture everything.
func listIndexes[T any](ctx context.Context, coll *mongo.Collection, target *T) error {
	ns := coll.Database().Name() + "." + coll.Name()

	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to start listing indexes for %#q: %w", ns, err)
	}
	err = cursor.All(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to list indexes for %#q: %w", ns, err)
	}

	return nil
}

func (s *DumpRestoreSuite) TestRestoreZeroTimestamp() {
	ctx := s.T().Context()

	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(ctx)
		if err != nil {
			s.T().Fatalf("Failed to drop test database: %v", err)
		}
	}()

	coll := testDB.Collection("mycoll")

	docID := bson.Timestamp{}

	_, err = coll.UpdateOne(
		ctx,
		bson.D{
			{"_id", docID},
		},
		mongo.Pipeline{
			{{"$replaceRoot", bson.D{
				{"newRoot", bson.D{
					{"$literal", bson.D{
						{"empty_time", bson.Timestamp{}},
						{"other", "$$ROOT"},
					}},
				}},
			}}},
		},
		mopt.UpdateOne().SetUpsert(true),
	)
	s.Require().NoError(err, "should insert (via update/upsert)")

	withBSONMongodumpForCollection(s.T(), coll.Database().Name(), coll.Name(), func(dir string) {
		restore, err := getRestoreWithArgs(
			mongorestore.DropOption,
			dir,
		)
		s.Require().NoError(err)
		defer restore.Close()

		result := restore.Restore()
		s.Require().NoError(result.Err, "can run mongorestore (result: %+v)", result)
		s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")
	})

	cursor, err := coll.Find(ctx, bson.D{})
	s.Require().NoError(err, "should find docs")
	docs := []bson.M{}
	s.Require().NoError(cursor.All(ctx, &docs), "should read docs")

	s.Require().Len(docs, 1, "expect docs count")
	assert.Equal(
		s.T(),
		bson.M{
			"_id":        docID,
			"empty_time": bson.Timestamp{},
			"other":      "$$ROOT",
		},
		docs[0],
		"expect empty timestamp restored",
	)
}

func (s *DumpRestoreSuite) TestRestoreZeroTimestamp_NonClobber() {
	ctx := s.T().Context()

	session, err := testutil.GetBareSession()
	s.Require().NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)
	defer func() {
		err = testDB.Drop(ctx)
		if err != nil {
			s.T().Fatalf("Failed to drop test database: %v", err)
		}
	}()

	coll := testDB.Collection("mycoll")

	docID := strings.Repeat("x", 7)

	_, err = coll.UpdateOne(
		ctx,
		bson.D{
			{"_id", docID},
		},
		mongo.Pipeline{
			{{"$replaceRoot", bson.D{
				{"newRoot", bson.D{
					{"empty_time", bson.Timestamp{}},
				}},
			}}},
		},
		mopt.UpdateOne().SetUpsert(true),
	)
	s.Require().NoError(err, "should insert (via update/upsert)")

	withBSONMongodumpForCollection(s.T(), coll.Database().Name(), coll.Name(), func(dir string) {
		updated, err := coll.UpdateOne(
			ctx,
			bson.D{
				{"_id", docID},
			},
			mongo.Pipeline{
				{{"$replaceRoot", bson.D{
					{"newRoot", bson.D{
						{"nonempty_time", bson.Timestamp{1, 2}},
					}},
				}}},
			},
		)
		s.Require().NoError(err, "should send update")
		s.Require().NotZero(updated.MatchedCount, "update should match a doc")

		restore, err := getRestoreWithArgs(
			dir,
		)
		s.Require().NoError(err)
		defer restore.Close()

		result := restore.Restore()
		s.Require().NoError(result.Err, "can run mongorestore")

		assert.EqualValues(s.T(), 1, result.Failures, "mongorestore reports failure")
	})

	cursor, err := coll.Find(ctx, bson.D{})
	s.Require().NoError(err, "should find docs")
	docs := []bson.M{}
	s.Require().NoError(cursor.All(ctx, &docs), "should read docs")

	s.Require().Len(docs, 1, "expect docs count")
	assert.NotContains(
		s.T(),
		docs[0],
		"empty_time",
		"restore did not clobber existing document (found: %+v)",
		docs[0],
	)
}

func (s *DumpRestoreSuite) TestRestoreMultipleIDIndexes() {
	cases := []struct {
		Label   string
		Indexes []mongo.IndexModel
	}{
		{
			Label: "single simple hashed ID index",
			Indexes: []mongo.IndexModel{
				{Keys: bson.D{{"_id", "hashed"}}},
			},
		},
		{
			Label: "multihashed collations 2dsphere",
			Indexes: []mongo.IndexModel{
				{Keys: bson.D{{"_id", "hashed"}}},
				{
					Keys: bson.D{{"_id", "hashed"}},
					Options: mopt.Index().
						SetName("_id_hashed_de").
						SetCollation(&mopt.Collation{Locale: "de"}),
				},
				{
					Keys: bson.D{{"_id", "hashed"}},
					Options: mopt.Index().
						SetName("_id_hashed_ar").
						SetCollation(&mopt.Collation{Locale: "ar"}),
				},
				{Keys: bson.D{{"_id", "2dsphere"}}},
			},
		},
	}

	dbName := uniqueDBName()

	for c := range cases {
		curCase := cases[c]
		indexesToCreate := curCase.Indexes

		s.Run(
			curCase.Label,
			func() {
				for attemptNum := range [20]any{} {
					s.Run(
						fmt.Sprintf("attempt %d", attemptNum),
						func() {
							session, err := testutil.GetBareSession()
							s.Require().NoError(err, "should connect to server")

							ctx := s.T().Context()

							testDB := session.Database(dbName)

							collName := strings.ReplaceAll(
								fmt.Sprintf("%s %d", curCase.Label, attemptNum),
								" ",
								"-",
							)
							coll := testDB.Collection(collName)

							_, err = coll.Indexes().CreateMany(ctx, indexesToCreate)
							s.Require().NoError(err, "indexes should be created")

							archivedIndexes := []bson.M{}
							s.Require().NoError(
								listIndexes(ctx, coll, &archivedIndexes),
								"should list indexes",
							)

							withBSONMongodumpForCollection(
								s.T(),
								testDB.Name(),
								coll.Name(),
								func(dir string) {
									restore, err := getRestoreWithArgs(
										mongorestore.DropOption,
										dir,
									)
									s.Require().NoError(err)
									defer restore.Close()

									result := restore.Restore()
									s.Require().NoError(
										result.Err,
										"%s: mongorestore should finish OK",
										curCase.Label,
									)
									s.Require().EqualValues(
										0,
										result.Failures,
										"%s: mongorestore should report 0 failures",
										curCase.Label,
									)
								},
							)

							restoredIndexes := []bson.M{}
							s.Require().NoError(
								listIndexes(ctx, coll, &restoredIndexes),
								"should list indexes",
							)

							assert.ElementsMatch(
								s.T(),
								archivedIndexes,
								restoredIndexes,
								"indexes should round-trip dump/restore (attempt #%d)",
								1+attemptNum,
							)
						},
					)
				}
			},
		)

	}
}

func TestRestoreUsersOrRoles(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
	session, err := testutil.GetBareSession()
	if err != nil {
		t.Fatalf("No server available")
	}

	Convey("With a test MongoRestore", t, func() {
		args := []string{
			mongorestore.NumParallelCollectionsOption, "1",
			mongorestore.NumInsertionWorkersOption, "1",
		}

		Convey("Restoring users and roles should drop tempusers and temproles", func() {

			restore, err := getRestoreWithArgs(args...)
			So(err, ShouldBeNil)
			defer restore.Close()

			db := session.Database("admin")

			restore.TargetDirectory = "../../mongorestore/testdata/usersdump"
			result := restore.Restore()
			So(result.Err, ShouldBeNil)

			adminCollections, err := db.ListCollectionNames(t.Context(), bson.M{})
			So(err, ShouldBeNil)

			for _, collName := range adminCollections {
				So(collName, ShouldNotEqual, "tempusers")
				So(collName, ShouldNotEqual, "temproles")
			}
		})

		Convey("If --dumpUsersAndRoles was not used with the target", func() {
			Convey("Restoring from db directory should not be allowed", func() {
				args = append(
					args,
					mongorestore.RestoreDBUsersAndRolesOption,
					mongorestore.DBOption,
					"db1",
					"../../mongorestore/testdata/testdirs/db1",
				)

				restore, err := getRestoreWithArgs(args...)
				So(err, ShouldBeNil)
				defer restore.Close()

				result := restore.Restore()
				So(errors.Is(result.Err, mongorestore.NoUsersOrRolesInDumpError), ShouldBeTrue)
			})

			Convey("Restoring from base dump directory should not be allowed", func() {
				args = append(
					args,
					mongorestore.RestoreDBUsersAndRolesOption,
					mongorestore.DBOption,
					"db1",
					"../../mongorestore/testdata/testdirs",
				)

				restore, err := getRestoreWithArgs(args...)
				So(err, ShouldBeNil)
				defer restore.Close()

				result := restore.Restore()
				So(errors.Is(result.Err, mongorestore.NoUsersOrRolesInDumpError), ShouldBeTrue)
			})

			Convey("Restoring from archive of entire dump should not be allowed", func() {
				withArchiveMongodump(t, func(archivePath string) {
					args = append(
						args,
						mongorestore.RestoreDBUsersAndRolesOption,
						mongorestore.DBOption,
						"db1",
						mongorestore.ArchiveOption+"="+archivePath,
					)

					restore, err := getRestoreWithArgs(args...)
					So(err, ShouldBeNil)
					defer restore.Close()

					result := restore.Restore()
					So(errors.Is(result.Err, mongorestore.NoUsersOrRolesInDumpError), ShouldBeTrue)

				})
			})
		})
	})
}

func TestUnversionedIndexes(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "no client available")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "get cluster version")

	dbName := t.Name()
	collName := "coll"

	coll := session.Database(dbName).Collection(collName)

	metadataEJSON, err := bson.MarshalExtJSON(
		bson.D{
			{"collectionName", collName},
			{"type", "collection"},
			{"uuid", uuid.New().String()},
			{"indexes", []bson.D{
				{
					{"v", 2},
					{"key", bson.D{{"_id", 1}}},
					{"name", "_id_"},
				},
				{
					{"v", 2},
					{"key", bson.D{{"myfield", "2dsphere"}}},
					{"name", "my2dsphere"},
				},
			}},
		},
		false,
		false,
	)
	require.NoError(t, err, "should marshal metadata to extJSON")

	testArchive := archive.SimpleArchive{
		Header: archive.Header{
			ServerVersion: serverVersion.String(),
		},
		CollectionMetadata: []archive.CollectionMetadata{
			{
				Database:   dbName,
				Collection: collName,
				Metadata:   string(metadataEJSON),
				Size:       0,
			},
		},
		Namespaces: []archive.SimpleNamespace{
			{
				Database:   dbName,
				Collection: collName,
			},
		},
	}
	archiveBytes, err := testArchive.Marshal()
	require.NoError(t, err, "should marshal the archive")

	withArchiveMongodump(t, func(archivePath string) {
		require.NoError(t, os.WriteFile(archivePath, archiveBytes, 0644))

		restore, err := getRestoreWithArgs(
			mongorestore.DropOption,
			mongorestore.ArchiveOption+"="+archivePath,
		)
		require.NoError(t, err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(t, result.Err, "can run mongorestore")
		require.EqualValues(t, 0, result.Failures, "mongorestore reports 0 failures")

		cursor, err := coll.Indexes().List(ctx)
		require.NoError(t, err, "should open index-list cursor")

		var indexes []idx.IndexDocument
		err = cursor.All(ctx, &indexes)
		require.NoError(t, err, "should fetch index specs")

		t.Logf("indexes: %+v", indexes)

		var twoDIndexDoc idx.IndexDocument

		for _, index := range indexes {
			if index.Options["name"] == "my2dsphere" {
				twoDIndexDoc = index
			}
		}

		require.NotNil(t, twoDIndexDoc.Key, "should find 2dsphere index (indexes: %+v)", indexes)
		assert.EqualValues(t, 1, twoDIndexDoc.Options["2dsphereIndexVersion"])
	})
}

func TestRestoreTimeseriesCollectionsWithMixedSchema(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "no cluster available")

	defer sessionProvider.Close()

	session, err := sessionProvider.GetSession()
	require.NoError(t, err, "no client available")

	fcv := testutil.GetFCV(session)
	// TODO: Enable tests for 6.0, 7.0 and 8.0 (TOOLS-3597).
	// The server fix for SERVER-84531 was only backported to 7.3.
	if cmp, err := testutil.CompareFCV(fcv, "7.3"); err != nil || cmp < 0 {
		require.NoError(t, err, "get fcv")
		t.Skip("Requires server with FCV 7.3 or later")
	}

	if cmp, err := testutil.CompareFCV(fcv, "8.0"); cmp >= 0 {
		require.NoError(t, err, "get fcv")
		t.Skip("The test currently fails on v8.0 because of SERVER-92222")
	}

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "parse server version")

	dbName := "timeseries_test_DB"
	collName := "timeseries_mixed_schema"
	testdb := session.Database(dbName)
	bucketColl := testdb.Collection(timeseriesCollName(serverVersion, collName))

	setupTimeseriesWithMixedSchema(t, dbName, collName)

	withArchiveMongodump(t, func(file string) {
		require.NoError(t, testdb.Collection(collName).Drop(ctx))
		require.NoError(t, bucketColl.Drop(ctx))

		restore, err := getRestoreWithArgs(
			mongorestore.DropOption,
			mongorestore.ArchiveOption+"="+file,
		)
		require.NoError(t, err)
		defer restore.Close()

		result := restore.Restore()
		require.NoError(t, result.Err, "can run mongorestore")
		require.EqualValues(t, 0, result.Failures, "mongorestore reports 0 failures")

		count, err := testdb.Collection(collName).CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.Equal(t, int64(2), count)

		count, err = bucketColl.CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.Equal(t, int64(1), count)

		hasMixedSchema := timeseriesBucketsMayHaveMixedSchemaData(t, bucketColl)
		require.True(t, hasMixedSchema)

		//nolint:errcheck
		defer testdb.Collection(collName).Drop(ctx)
	})
}

func timeseriesBucketsMayHaveMixedSchemaData(
	t *testing.T,
	bucketColl *mongo.Collection,
) bool {
	ctx := t.Context()
	cursor, err := bucketColl.Database().RunCommandCursor(ctx, bson.D{
		{"aggregate", bucketColl.Name()},
		{"pipeline", bson.A{
			bson.D{{"$listCatalog", bson.D{}}},
		}},
		{"readConcern", bson.D{{"level", "majority"}}},
		{"cursor", bson.D{}},
	})
	require.NoError(t, err)

	if !cursor.Next(ctx) {
		require.Fail(t, "no entry in $listCatalog response")
	}

	md, err := cursor.Current.LookupErr("md")
	require.NoError(t, err, "lookup 'md' field")

	hasMixedSchema, err := md.Document().LookupErr("timeseriesBucketsMayHaveMixedSchemaData")
	require.NoError(t, err, "lookup 'timeseriesBucketsMayHaveMixedSchemaData' field")

	return hasMixedSchema.Boolean()
}

func setupTimeseriesWithMixedSchema(t *testing.T, dbName string, collName string) {
	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err, "get session provider")

	serverVersion, err := sessionProvider.ServerVersionArray()
	require.NoError(t, err, "get server version")

	client, err := sessionProvider.GetSession()
	require.NoError(t, err, "get session")

	err = client.Database(dbName).Collection(collName).Drop(t.Context())
	require.NoError(t, err, "drop existing coll")

	createCmd := bson.D{
		{"create", collName},
		{"timeseries", bson.D{
			{"timeField", "t"},
			{"metaField", "m"},
		}},
	}

	createRes := sessionProvider.DB(dbName).RunCommand(t.Context(), createCmd)
	require.NoError(t, createRes.Err(), "create timeseries coll")

	// SERVER-84531 was only backported to 7.3.
	if cmp, err := testutil.CompareFCV(testutil.GetFCV(client), "7.3"); err != nil || cmp >= 0 {
		res := sessionProvider.DB(dbName).RunCommand(t.Context(), bson.D{
			{"collMod", collName},
			{"timeseriesBucketsMayHaveMixedSchemaData", true},
		})

		require.NoError(t, res.Err(), "collMod timeseries collection")
	}

	bucketName := timeseriesCollName(serverVersion, collName)
	bucketColl := sessionProvider.DB(dbName).Collection(bucketName)
	bucketJSON := `{"_id":{"$oid":"65a6eb806ffc9fa4280ecac4"},"control":{"version":1,"min":{"_id":{"$oid":"65a6eba7e6d2e848e08c3750"},"t":{"$date":"2024-01-16T20:48:00Z"},"a":1},"max":{"_id":{"$oid":"65a6eba7e6d2e848e08c3751"},"t":{"$date":"2024-01-16T20:48:39.448Z"},"a":"a"}},"meta":0,"data":{"_id":{"0":{"$oid":"65a6eba7e6d2e848e08c3750"},"1":{"$oid":"65a6eba7e6d2e848e08c3751"}},"t":{"0":{"$date":"2024-01-16T20:48:39.448Z"},"1":{"$date":"2024-01-16T20:48:39.448Z"}},"a":{"0":"a","1":1}}}`
	var bucketMap map[string]any
	err = json.Unmarshal([]byte(bucketJSON), &bucketMap)
	require.NoError(t, err, "unmarshal json")

	err = bsonutil.ConvertLegacyExtJSONDocumentToBSON(bucketMap)
	require.NoError(t, err, "convert extjson to bson")

	_, err = bucketColl.InsertOne(t.Context(), bucketMap)
	require.NoError(t, err, "insert bucket doc")
}

func timeseriesCollName(version db.Version, base string) string {
	if version.SupportsRawData() {
		// viewless timeseries
		return base
	}

	return common.TimeseriesBucketPrefix + base
}

func TestIgnoreMongoDBInternal(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	sessionProvider, _, err := testutil.GetBareSessionProvider()
	require.NoError(t, err)

	if ok, _ := sessionProvider.IsReplicaSet(); !ok {
		t.Skip("replica set required")
	}

	ctx := t.Context()

	dbName := util.MongoDBInternalDBPrefix + t.Name()

	client, err := testutil.GetBareSession()
	require.NoError(t, err, "must connect to server")

	internalColl := client.Database(dbName).Collection(t.Name())

	_, err = internalColl.InsertOne(ctx, bson.D{{"_id", 1}})
	require.NoError(t, err, "must write to the internal DB")

	_, err = client.Database(t.Name()).Collection(t.Name()).InsertOne(ctx, bson.D{})
	require.NoError(t, err, "must write to the user DB")

	// Send repeated updates to the internal DB in a background thread.
	writesCtx, writesCancel := context.WithCancelCause(ctx)
	updatesDone := make(chan struct{})
	go func() {
		defer close(updatesDone)

		for writesCtx.Err() == nil {
			_, err := internalColl.InsertOne(
				writesCtx,
				bson.D{},
			)

			if !errors.Is(err, context.Canceled) {
				require.NoError(t, err, "must write to the internal DB")
			}
		}

		t.Logf("Updates canceled: %v", context.Cause(writesCtx))
	}()

	withArchiveMongodump(
		t,
		func(archivePath string) {
			writesCancel(fmt.Errorf("archive is finished"))
			<-updatesDone

			require.NoError(t, client.Database(internalColl.Database().Name()).Drop(ctx))
			require.NoError(t, client.Database(t.Name()).Drop(ctx))

			restore, err := getRestoreWithArgs(
				mongorestore.ArchiveOption+"="+archivePath,
				"-vv",
				"--oplogReplay",
				"--drop",
			)
			require.NoError(t, err)
			defer restore.Close()

			result := restore.Restore()
			require.NoError(t, result.Err, "can run mongorestore")
			require.EqualValues(
				t,
				0,
				result.Failures,
				"mongorestore reports 0 failures (result=%+v)",
				result,
			)
		},
		"--oplog",
		"-vv",
	)

	dbNames, err := client.ListDatabaseNames(ctx, bson.D{})
	require.NoError(t, err)

	assert.Contains(t, dbNames, t.Name(), "user DB restored")
	assert.NotContains(t, dbNames, internalColl.Database().Name(), "internal DB ignored")
}

func TestFinalNewlinesInNamespaces(t *testing.T) {
	testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

	ctx := t.Context()
	require := require.New(t)

	session, err := testutil.GetBareSession()
	require.NoError(err, "can connect to server")

	allNames := []string{
		"no-nl",
		"\ninitial-nl",
		"mid-\n-nl",
		"final-nl\n",
		"\ninitial-and-final-nl\n",
		"\nnl-\n-everywhere\n",
	}

	nlVariants := []struct {
		label string
		nl    string
	}{
		{"LF", "\n"},
		{"CR", "\r"},
		{"CRLF", "\r\n"},
	}

	for _, variant := range nlVariants {
		myAllNames := lo.Map(
			allNames,
			func(name string, _ int) string {
				return strings.ReplaceAll(name, "\n", variant.nl)
			},
		)

		t.Run(
			variant.label,
			func(t *testing.T) {
				for _, dbname := range myAllNames {
					t.Run(
						fmt.Sprintf("dbname=%s", strconv.Quote(dbname)),
						func(t *testing.T) {
							require.NoError(session.Database(dbname).Drop(ctx))
							createCollectionsWithTestDocuments(
								t,
								session.Database(dbname),
								myAllNames,
							)

							withArchiveMongodump(t, func(archivePath string) {
								require.NoError(session.Database(dbname).Drop(ctx))

								colls, err := session.Database(dbname).
									ListCollectionNames(ctx, bson.D{})
								require.NoError(err)
								require.Empty(colls, "sanity: db drop should drop all collections")

								restore, err := getRestoreWithArgs(
									mongorestore.DBOption, dbname,
									mongorestore.ArchiveOption+"="+archivePath,
									"-vv",
								)
								require.NoError(err)
								defer restore.Close()

								result := restore.Restore()
								require.NoError(result.Err, "can run mongorestore")
								require.EqualValues(
									0,
									result.Failures,
									"mongorestore reports 0 failures (result=%+v)",
									result,
								)
							})

							colls, err := session.Database(dbname).
								ListCollectionNames(ctx, bson.D{})
							require.NoError(err)

							assert.ElementsMatch(t, myAllNames, colls, "all collections restored")
						},
					)
				}
			},
		)
	}

}

func withArchiveMongodump(t *testing.T, testCase func(string), dumpArgs ...string) {
	dir, cleanup := testutil.MakeTempDir(t)
	defer cleanup()
	file := filepath.Join(dir, "archive")
	runArchiveMongodump(t, file, dumpArgs...)
	testCase(file)
}

func runArchiveMongodump(t *testing.T, file string, dumpArgs ...string) string {
	require := require.New(t)
	runMongodumpWithArgs(
		t,
		append(
			[]string{mongorestore.ArchiveOption + "=" + file},
			dumpArgs...,
		)...,
	)
	_, err := os.Stat(file)
	require.NoError(err, "dump created archive data file")
	return file
}
