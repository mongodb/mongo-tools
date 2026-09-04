package dumprestore

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/db/dsctest"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/testopts"
	"github.com/mongodb/mongo-tools/common/testutil"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongodump"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/sync/errgroup"
)

// dbNameCounter makes uniqueDBName distinct on every call, so that a round-trip
// test run in both cluster orientations never reuses a name on the shared
// clusters.
var dbNameCounter atomic.Int64

func uniqueDBName() string {
	return fmt.Sprintf(
		"mongorestore_test_%d_%d_%d",
		os.Getpid(),
		time.Now().UnixMilli(),
		dbNameCounter.Add(1),
	)
}

func (s *DumpRestoreSuite) TestPipedDumpRestore() {
	s.withOrientations(func(cc crossCluster) {
		s.T().Logf("start %#q", s.T().Name())
		ctx := s.Context()

		srcCollNames := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

		db := cc.source.Database(uniqueDBName())

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

			dump, err := getArchiveMongoDumpForURI(s.T(), cc.sourceURI, writer)
			if err != nil {
				return errors.Wrap(err, "create mongodump")
			}

			dump.ToolOptions.DB = db.Name()

			s.Assert().NoError(dump.Dump(), "dump should work")

			return nil
		})

		eg.Go(func() error {
			defer reader.Close()

			restore, err := getArchiveMongoRestoreForURI(s.T(), cc.targetURI, reader)
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

			s.Assert().NoError(restore.Restore().Err, "restore should work")

			return nil
		})

		s.Require().NoError(eg.Wait())
	})
}

func (s *DumpRestoreSuite) TestDumpAndRestoreConfigDB() {
	_, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to server")

	s.Run(
		"dump config db includes all collections",
		s.testDumpAndRestoreConfigDBIncludesAllCollections,
	)

	s.Run(
		"dump all dbs includes only some config collections",
		s.testDumpAndRestoreAllDBsIgnoresSomeConfigCollections,
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

func (s *DumpRestoreSuite) testDumpAndRestoreConfigDBIncludesAllCollections() {
	s.withOrientations(func(cc crossCluster) {
		configDB := cc.source.Database("config")

		collections := s.createCollectionsWithTestDocuments(
			configDB,
			append(configCollectionNamesToKeep, userDefinedConfigCollectionNames...),
		)
		defer s.clearDB(configDB)

		s.withBSONMongodumpForURI(
			cc.sourceURI,
			func(dir string) {
				s.clearDB(configDB)

				restore, err := getRestoreWithArgsForURI(cc.targetURI, dir)
				s.Require().NoError(err)
				defer restore.Close()

				result := restore.Restore()
				s.Require().NoError(result.Err, "can run mongorestore")
				s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

				for _, collection := range collections {
					targetColl := cc.target.Database("config").Collection(collection.Name())
					r := targetColl.FindOne(s.Context(), testDocument)
					s.Require().NoError(r.Err(), "expected document")
				}
			},
			"--db", "config",
			"--excludeCollection", "transactions",
		)
	})
}

func (s *DumpRestoreSuite) testDumpAndRestoreAllDBsIgnoresSomeConfigCollections() {
	s.withOrientations(func(cc crossCluster) {
		// Drop any databases that other tests may have left behind with validators
		// that would cause failures during the full dump+restore.
		s.Require().NoError(cc.source.Database("mongodump_test_db").Drop(s.Context()))

		configDB := cc.source.Database("config")

		userDefinedCollections := s.createCollectionsWithTestDocuments(
			configDB,
			userDefinedConfigCollectionNames,
		)
		collectionsToKeep := s.createCollectionsWithTestDocuments(
			configDB,
			configCollectionNamesToKeep,
		)
		defer s.clearDB(configDB)

		s.withBSONMongodumpForURI(
			cc.sourceURI,
			func(dir string) {
				s.clearDB(configDB)

				restore, err := getRestoreWithArgsForURI(
					cc.targetURI,
					mongorestore.DropOption,
					dir,
				)
				s.Require().NoError(err)
				defer restore.Close()

				result := restore.Restore()
				s.Require().NoError(result.Err, "can run mongorestore")
				s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

				for _, collection := range collectionsToKeep {
					targetColl := cc.target.Database("config").Collection(collection.Name())
					r := targetColl.FindOne(s.Context(), testDocument)
					s.Require().NoError(r.Err(), "expected document")
				}

				for _, collection := range userDefinedCollections {
					targetColl := cc.target.Database("config").Collection(collection.Name())
					r := targetColl.FindOne(s.Context(), testDocument)
					s.Require().Error(r.Err(), "expected no document")
				}
			},
		)
	})
}

func getRestoreWithArgs(additionalArgs ...string) (*mongorestore.MongoRestore, error) {
	return getRestoreWithArgsForURI(os.Getenv(testopts.URIEnvVar), additionalArgs...)
}

func getRestoreWithArgsForURI(
	uri string,
	additionalArgs ...string,
) (*mongorestore.MongoRestore, error) {
	opts, err := mongorestore.ParseOptions(
		append(testopts.GetBareArgsForURI(uri), additionalArgs...),
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

func getArchiveMongoDumpForURI(
	t *testing.T,
	uri string,
	output io.WriteCloser,
) (*mongodump.MongoDump, error) {
	provider, toolOpts, err := testutil.GetBareSessionProviderForURI(t, uri)
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

func getArchiveMongoRestoreForURI(
	t *testing.T,
	uri string,
	input io.ReadCloser,
) (*mongorestore.MongoRestore, error) {
	_, toolOpts, err := testutil.GetBareSessionProviderForURI(t, uri)
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
	s.withOrientations(func(cc crossCluster) {
		ctx := s.Context()

		dbName := uniqueDBName()
		testDB := cc.source.Database(dbName)

		coll := testDB.Collection("mycoll")

		docID := bson.Timestamp{}

		_, err := coll.UpdateOne(
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

		s.withBSONMongodumpForCollectionForURI(
			cc.sourceURI,
			coll.Database().Name(),
			coll.Name(),
			func(dir string) {
				restore, err := getRestoreWithArgsForURI(
					cc.targetURI,
					mongorestore.DropOption,
					dir,
				)
				s.Require().NoError(err)
				defer restore.Close()

				result := restore.Restore()
				s.Require().NoError(result.Err, "can run mongorestore (result: %+v)", result)
				s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")
			},
		)

		targetColl := cc.target.Database(dbName).Collection("mycoll")

		cursor, err := targetColl.Find(ctx, bson.D{})
		s.Require().NoError(err, "should find docs")
		docs := []bson.M{}
		s.Require().NoError(cursor.All(ctx, &docs), "should read docs")

		s.Require().Len(docs, 1, "expect docs count")
		s.Assert().Equal(
			bson.M{
				"_id":        docID,
				"empty_time": bson.Timestamp{},
				"other":      "$$ROOT",
			},
			docs[0],
			"expect empty timestamp restored",
		)
	})
}

func (s *DumpRestoreSuite) TestRestoreZeroTimestamp_NonClobber() {
	if testopts.SecondURI() != "" {
		s.T().Skip(
			"restore is expected to conflict with pre-existing data on the same cluster, " +
				"which a cross-cluster run (empty target) cannot produce",
		)
	}

	ctx := s.Context()

	session, err := testutil.GetBareSession(s.T())
	s.Require().NoError(err, "can connect to server")

	dbName := uniqueDBName()
	testDB := session.Database(dbName)

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

	s.withBSONMongodumpForCollection(coll.Database().Name(), coll.Name(), func(dir string) {
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

		s.Assert().EqualValues(1, result.Failures, "mongorestore reports failure")
	})

	cursor, err := coll.Find(ctx, bson.D{})
	s.Require().NoError(err, "should find docs")
	docs := []bson.M{}
	s.Require().NoError(cursor.All(ctx, &docs), "should read docs")

	s.Require().Len(docs, 1, "expect docs count")
	s.Assert().NotContains(
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

	s.withOrientations(func(cc crossCluster) {
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
								dbName := uniqueDBName()
								ctx := s.Context()

								testDB := cc.source.Database(dbName)

								collName := strings.ReplaceAll(
									fmt.Sprintf("%s %d", curCase.Label, attemptNum),
									" ",
									"-",
								)
								coll := testDB.Collection(collName)

								_, err := coll.Indexes().CreateMany(ctx, indexesToCreate)
								s.Require().NoError(err, "indexes should be created")

								archivedIndexes := []bson.M{}
								s.Require().NoError(
									listIndexes(ctx, coll, &archivedIndexes),
									"should list indexes",
								)

								s.withBSONMongodumpForCollectionForURI(
									cc.sourceURI,
									testDB.Name(),
									coll.Name(),
									func(dir string) {
										restore, err := getRestoreWithArgsForURI(
											cc.targetURI,
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

								targetColl := cc.target.Database(dbName).Collection(collName)
								restoredIndexes := []bson.M{}
								s.Require().NoError(
									listIndexes(ctx, targetColl, &restoredIndexes),
									"should list indexes",
								)

								s.Assert().ElementsMatch(
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
	})
}
func (s *DumpRestoreSuite) TestRestoreUsersOrRoles() {
	s.withOrientations(func(cc crossCluster) {
		s.Run("drops tempusers and temproles", func() {
			restore, err := getRestoreWithArgsForURI(
				cc.targetURI,
				mongorestore.NumParallelCollectionsOption, "1",
				mongorestore.NumInsertionWorkersOption, "1",
			)
			s.Require().NoError(err)
			defer restore.Close()

			adminDB := cc.target.Database("admin")
			restore.TargetDirectory = "../../mongorestore/testdata/usersdump"
			result := restore.Restore()
			s.Require().NoError(result.Err, "can run mongorestore")

			adminCollections, err := adminDB.ListCollectionNames(s.Context(), bson.M{})
			s.Require().NoError(err, "can list admin collections")

			for _, collName := range adminCollections {
				s.Assert().
					NotEqual("tempusers", collName, "tempusers should not exist after restore")
				s.Assert().
					NotEqual("temproles", collName, "temproles should not exist after restore")
			}
		})

		s.Run("without --dumpUsersAndRoles", func() {
			s.Run("db directory restore fails", func() {
				restore, err := getRestoreWithArgsForURI(
					cc.targetURI,
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

			s.Run("base dump directory restore fails", func() {
				restore, err := getRestoreWithArgsForURI(
					cc.targetURI,
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

			s.Run("archive of entire dump restore fails", func() {
				s.withArchiveMongodumpForURI(cc.sourceURI, func(archivePath string) {
					restore, err := getRestoreWithArgsForURI(
						cc.targetURI,
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
	})
}

func (s *DumpRestoreSuite) TestUnversionedIndexes() {
	s.withOrientations(func(cc crossCluster) {
		ctx := s.Context()

		sessionProvider, _, err := testutil.GetBareSessionProviderForURI(s.T(), cc.sourceURI)
		s.Require().NoError(err, "no source cluster available")

		serverVersion, err := sessionProvider.ServerVersionArray()
		s.Require().NoError(err, "get cluster version")

		dbName := uniqueDBName()
		collName := "coll"

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

		s.withArchiveMongodumpForURI(cc.sourceURI, func(archivePath string) {
			s.Require().NoError(os.WriteFile(archivePath, archiveBytes, 0644))

			restore, err := getRestoreWithArgsForURI(
				cc.targetURI,
				mongorestore.DropOption,
				mongorestore.ArchiveOption+"="+archivePath,
			)
			s.Require().NoError(err)
			defer restore.Close()

			result := restore.Restore()
			s.Require().NoError(result.Err, "can run mongorestore")
			s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

			targetColl := cc.target.Database(dbName).Collection(collName)
			cursor, err := targetColl.Indexes().List(ctx)
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

			s.Require().
				NotNil(twoDIndexDoc.Key, "should find 2dsphere index (indexes: %+v)", indexes)
			s.Assert().Equal(
				int32(1),
				twoDIndexDoc.Options["2dsphereIndexVersion"],
				"should have version 1 2dsphere index (unversioned)",
			)
		})
	})
}

func (s *DumpRestoreSuite) TestRestoreTimeseriesCollectionsWithMixedSchema() {
	s.withOrientations(func(cc crossCluster) {
		ctx := s.Context()

		sessionProvider, _, err := testutil.GetBareSessionProviderForURI(s.T(), cc.sourceURI)
		s.Require().NoError(err, "no source cluster available")

		fcv := testutil.GetFCV(cc.source)
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

		dbName := uniqueDBName()
		collName := "timeseries_mixed_schema"

		s.setupTimeseriesWithMixedSchema(dbName, collName)

		s.withArchiveMongodumpForURI(cc.sourceURI, func(file string) {
			targetDB := cc.target.Database(dbName)
			targetBucketColl := targetDB.Collection(timeseriesCollName(serverVersion, collName))

			s.Require().NoError(targetDB.Collection(collName).Drop(ctx))
			s.Require().NoError(targetBucketColl.Drop(ctx))

			restore, err := getRestoreWithArgsForURI(
				cc.targetURI,
				mongorestore.DropOption,
				mongorestore.ArchiveOption+"="+file,
			)
			s.Require().NoError(err)
			defer restore.Close()

			result := restore.Restore()
			s.Require().NoError(result.Err, "can run mongorestore")
			s.Require().EqualValues(0, result.Failures, "mongorestore reports 0 failures")

			count, err := targetDB.Collection(collName).CountDocuments(ctx, bson.M{})
			s.Require().NoError(err)
			s.Require().Equal(int64(2), count, "should have 2 documents in timeseries collection")

			count, err = targetBucketColl.CountDocuments(ctx, bson.M{})
			s.Require().NoError(err)
			s.Require().Equal(int64(1), count, "should have 1 document in bucket collection")

			hasMixedSchema := s.timeseriesBucketsMayHaveMixedSchemaData(targetBucketColl)
			s.Require().True(hasMixedSchema, "bucket collection should have mixed schema flag set")

			//nolint:errcheck
			defer targetDB.Collection(collName).Drop(ctx)
		})
	})
}

func (s *DumpRestoreSuite) TestIgnoreMongoDBInternal() {
	s.withOrientations(func(cc crossCluster) {
		sourceProvider, _, err := testutil.GetBareSessionProviderForURI(s.T(), cc.sourceURI)
		s.Require().NoError(err, "no source cluster available")

		targetProvider, _, err := testutil.GetBareSessionProviderForURI(s.T(), cc.targetURI)
		s.Require().NoError(err, "no target cluster available")

		if ok, _ := sourceProvider.IsReplicaSet(); !ok {
			s.T().Skip("replica set required for a --oplog dump")
		}
		if ok, _ := targetProvider.IsReplicaSet(); !ok {
			s.T().Skip("replica set required for --oplogReplay")
		}

		// Replaying an oplog runs applyOps, which DSC does not support, so the
		// skip keys off the restore target rather than the primary cluster.
		dsctest.SkipForDisaggregatedStorage(
			s.T(),
			cc.target,
			"it replays an oplog, and DSC does not support the applyOps command",
		)

		ctx := s.Context()

		testName := uniqueDBName()
		dbName := util.MongoDBInternalDBPrefix + testName

		client := cc.source

		internalColl := client.Database(dbName).Collection(testName)

		_, err = internalColl.InsertOne(ctx, bson.D{})
		s.Require().NoError(err, "must write to the internal DB")

		_, err = client.Database(testName).Collection(testName).InsertOne(ctx, bson.D{})
		s.Require().NoError(err, "must write to the user DB")

		writesCtx, writesCancel := context.WithCancelCause(ctx)
		updatesDone := make(chan struct{})
		go func() {
			defer close(updatesDone)
			currId := int32(0)

			for writesCtx.Err() == nil {
				_, err := internalColl.InsertOne(
					writesCtx,
					bson.D{{"_id", currId}},
				)
				currId++

				if !errors.Is(err, context.Canceled) {
					s.Require().NoError(err, "must write to the internal DB")
				}

				// Throttle inserts to reduce the chance of oplog rolling over.
				time.Sleep(10 * time.Millisecond)
			}

			s.T().Logf("Updates canceled: %v", context.Cause(writesCtx))
		}()

		s.withArchiveMongodumpForURI(
			cc.sourceURI,
			func(archivePath string) {
				writesCancel(fmt.Errorf("archive is finished"))
				<-updatesDone

				s.Require().NoError(cc.target.Database(internalColl.Database().Name()).Drop(ctx))
				s.Require().NoError(cc.target.Database(testName).Drop(ctx))

				restore, err := getRestoreWithArgsForURI(
					cc.targetURI,
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

		dbNames, err := cc.target.ListDatabaseNames(ctx, bson.D{})
		s.Require().NoError(err)

		s.Assert().Contains(dbNames, testName, "user DB restored")
		s.Assert().NotContains(dbNames, internalColl.Database().Name(), "internal DB ignored")
	})
}

func (s *DumpRestoreSuite) TestFinalNewlinesInNamespaces() {
	s.withOrientations(func(cc crossCluster) {
		ctx := s.Context()

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
								s.Require().NoError(cc.source.Database(dbname).Drop(ctx))
								s.createCollectionsWithTestDocuments(
									cc.source.Database(dbname),
									myAllNames,
								)

								s.withArchiveMongodumpForURI(
									cc.sourceURI,
									func(archivePath string) {
										s.Require().NoError(cc.source.Database(dbname).Drop(ctx))

										colls, err := cc.source.Database(dbname).
											ListCollectionNames(ctx, bson.D{})
										s.Require().NoError(err)
										s.Require().
											Empty(colls, "sanity: db drop should drop all collections")

										restore, err := getRestoreWithArgsForURI(
											cc.targetURI,
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
									},
								)

								colls, err := cc.target.Database(dbname).
									ListCollectionNames(ctx, bson.D{})
								s.Require().NoError(err)

								s.Assert().ElementsMatch(
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
	})
}

func timeseriesCollName(version db.Version, base string) string {
	if version.SupportsRawData() {
		return base
	}

	return common.TimeseriesBucketPrefix + base
}
