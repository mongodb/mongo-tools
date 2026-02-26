// This tool tests the ability of mongodump and mongorestore to move data between clusters with and
// without replicated record IDs enabled.
//
// It performs a full dump/restore cycle between two clusters. When both clusters are of the same
// type (both with or both without recordIdsReplicated), it verifies the data matches after
// restore. When the clusters differ (one with and one without recordIdsReplicated), it verifies
// that mongorestore fails when --oplogReplay is used.
//
// Usage:
//
//	go run main.go --source=<uri> --destination=<uri> [--keep-dump]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName          = "rir_test_db"
	docsPerCollection   = 50
	concurrentOpsDBName = "rir_test_concurrent_db"
	concurrentOpsColl   = "concurrent_coll"
	dumpStartDelay      = 2 * time.Second
	concurrentOpsPeriod = 5 * time.Second
)

// collectionSpec defines a collection to create for testing.
type collectionSpec struct {
	name       string
	options    *options.CreateCollectionOptionsBuilder
	indexes    []mongo.IndexModel
	timeseries *options.TimeSeriesOptionsBuilder
}

// testContext holds the state needed throughout the test.
type testContext struct {
	srcURI                        string
	dstURI                        string
	keepDump                      bool
	srcClient                     *mongo.Client
	dstClient                     *mongo.Client
	dumpDir                       string
	simpleDumpDir                 string
	barrierFile                   string
	srcRecordIdsReplicatedEnabled bool
	dstRecordIdsReplicatedEnabled bool
}

func main() {
	srcURI := flag.String("source", "", "MongoDB URI for source cluster (required)")
	dstURI := flag.String("destination", "", "MongoDB URI for destination cluster (required)")
	keepDump := flag.Bool("keep-dump", false, "Keep the dump directory for inspection")
	flag.Parse()

	if *srcURI == "" || *dstURI == "" {
		flag.Usage()
		log.Fatal("Both --source and --destination are required")
	}

	if err := run(*srcURI, *dstURI, *keepDump); err != nil {
		log.Fatalf("Test failed: %v", err)
	}

	log.Println("✅ Test completed successfully!")
}

func run(srcURI, dstURI string, keepDump bool) error {
	ctx := context.Background()

	tc := &testContext{
		srcURI:   srcURI,
		dstURI:   dstURI,
		keepDump: keepDump,
	}

	if err := tc.buildBinaries(); err != nil {
		return err
	}

	if err := tc.connectToClusters(ctx); err != nil {
		return err
	}

	defer tc.disconnectClients(ctx)

	if err := tc.cleanTestDBs(ctx); err != nil {
		return err
	}

	if err := tc.setupSourceCluster(ctx); err != nil {
		return err
	}

	if err := tc.createDumpDir(); err != nil {
		return err
	}
	if !keepDump {
		defer tc.removeDumpDir()
	}

	if err := tc.runDumpWithConcurrentOps(ctx); err != nil {
		return err
	}

	if err := tc.removeSystemDBsFromDump(); err != nil {
		return err
	}

	if err := tc.verifyOplogHasEntries(); err != nil {
		return err
	}

	if tc.srcRecordIdsReplicatedEnabled != tc.dstRecordIdsReplicatedEnabled {
		// Cycle 1: with oplog replay — expect failure due to cluster type mismatch.
		log.Println(
			"Source and destination clusters differ in recordIdsReplicated support; verifying that mongorestore fails with --oplogReplay...",
		)
		if err := tc.runRestoreExpectingFailure(); err != nil {
			return err
		}

		// Cycle 2: without oplog replay — expect success.
		log.Println("Verifying that mongorestore succeeds without --oplogReplay...")
		if err := tc.cleanTestDBs(ctx); err != nil {
			return err
		}
		if err := tc.runDumpWithoutOplog(); err != nil {
			return err
		}
		defer tc.removeSimpleDumpDir()

		if err := tc.removeSystemDBsFromSimpleDump(); err != nil {
			return err
		}
		if err := tc.runRestoreWithoutOplog(); err != nil {
			return err
		}
		if err := tc.verifyClustersMatch(ctx); err != nil {
			return err
		}
	} else {
		if err := tc.runRestore(); err != nil {
			return err
		}

		if err := tc.verifyClustersMatch(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (tc *testContext) buildBinaries() error {
	log.Println("Building mongodump and mongorestore binaries...")
	cmd := exec.Command("go", "run", "build.go", "build", "-pkgs=mongodump,mongorestore")
	cmd.Dir = repoRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build binaries: %w", err)
	}
	log.Println("✅ Binaries built successfully")
	return nil
}

func (tc *testContext) connectToClusters(ctx context.Context) error {
	log.Println("Connecting to source cluster...")
	srcClient, err := mongo.Connect(options.Client().ApplyURI(tc.srcURI))
	if err != nil {
		return fmt.Errorf("failed to connect to source: %w", err)
	}
	if err := srcClient.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping source: %w", err)
	}
	tc.srcClient = srcClient
	log.Println("✅ Connected to source cluster")

	log.Println("Connecting to destination cluster...")
	dstClient, err := mongo.Connect(options.Client().ApplyURI(tc.dstURI))
	if err != nil {
		return fmt.Errorf("failed to connect to destination: %w", err)
	}
	if err := dstClient.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping destination: %w", err)
	}
	tc.dstClient = dstClient
	log.Println("✅ Connected to destination cluster")

	tc.srcRecordIdsReplicatedEnabled, err = hasRecordIdsReplicated(ctx, srcClient)
	if err != nil {
		return fmt.Errorf("failed to check recordIdsReplicated on source: %w", err)
	}
	log.Printf("Source cluster recordIdsReplicated: %v", tc.srcRecordIdsReplicatedEnabled)

	tc.dstRecordIdsReplicatedEnabled, err = hasRecordIdsReplicated(ctx, dstClient)
	if err != nil {
		return fmt.Errorf("failed to check recordIdsReplicated on destination: %w", err)
	}
	log.Printf("Destination cluster recordIdsReplicated: %v", tc.dstRecordIdsReplicatedEnabled)

	return nil
}

// hasRecordIdsReplicated checks whether the given cluster has the featureFlagRecordIdsReplicated
// parameter enabled.
func hasRecordIdsReplicated(ctx context.Context, client *mongo.Client) (bool, error) {
	result := client.Database("admin").RunCommand(
		ctx,
		bson.D{
			{"getParameter", 1},
			{"featureFlagRecordIdsReplicated", 1},
		},
	)
	if result.Err() != nil {
		// This is the "InvalidOptions" error code, which is what we get when the parameter name
		// isn't recognized by the Server.
		if slices.Contains(mongo.ErrorCodes(result.Err()), 72) {
			// If the command fails, the feature flag doesn't exist or isn't enabled. Or maybe something
			// is totally broken, in which case we will find that out when we attempt other DB
			// operations.
			return false, nil
		}
		return false, result.Err()
	}

	var doc struct {
		FeatureFlagRecordIdsReplicated struct {
			Value bool `bson:"value"`
		} `bson:"featureFlagRecordIdsReplicated"`
	}
	if err := result.Decode(&doc); err != nil {
		return false, nil
	}

	return doc.FeatureFlagRecordIdsReplicated.Value, nil
}

func (tc *testContext) disconnectClients(ctx context.Context) {
	if err := tc.srcClient.Disconnect(ctx); err != nil {
		log.Println("❌ Failed to disconnect source client: %v", err)
	}
	if err := tc.dstClient.Disconnect(ctx); err != nil {
		log.Println("❌ Failed to disconnect destination client: %v", err)
	}
}

func (tc *testContext) cleanTestDBs(ctx context.Context) error {
	log.Println("Cleaning test databases...")

	for _, db := range []string{testDBName, concurrentOpsDBName} {
		log.Printf("  Dropping %s on source...", db)
		if err := tc.srcClient.Database(db).Drop(ctx); err != nil {
			return fmt.Errorf("failed to drop %#q db on the source: %w", db, err)
		}

		log.Printf("  Dropping %s on destination...", db)
		if err := tc.dstClient.Database(db).Drop(ctx); err != nil {
			return fmt.Errorf("failed to drop %#q db on the destination: %w", db, err)
		}
	}

	log.Println("✅ Test databases cleaned")
	return nil
}

func (tc *testContext) setupSourceCluster(ctx context.Context) error {
	log.Println("Setting up test collections on source...")

	db := tc.srcClient.Database(testDBName)

	for _, spec := range collectionSpecs() {
		log.Printf("  Creating collection: %s", spec.name)

		createOpts := spec.options
		if createOpts == nil {
			createOpts = options.CreateCollection()
		}
		if spec.timeseries != nil {
			createOpts.SetTimeSeriesOptions(spec.timeseries)
		}

		if err := db.CreateCollection(ctx, spec.name, createOpts); err != nil {
			return fmt.Errorf("failed to create collection %s: %w", spec.name, err)
		}

		if len(spec.indexes) > 0 {
			coll := db.Collection(spec.name)
			if _, err := coll.Indexes().CreateMany(ctx, spec.indexes); err != nil {
				return fmt.Errorf("failed to create indexes for %s: %w", spec.name, err)
			}
		}

		if err := insertTestDocuments(ctx, db.Collection(spec.name), spec); err != nil {
			return fmt.Errorf("failed to insert documents into %s: %w", spec.name, err)
		}
	}

	log.Println("✅ Test collections created and populated")
	return nil
}

func collectionSpecs() []collectionSpec {
	return []collectionSpec{
		{
			name: "simple_collection",
		},
		{
			name: "single_field_index",
			indexes: []mongo.IndexModel{
				{Keys: bson.D{{"field_a", 1}}},
			},
		},
		{
			name: "compound_index",
			indexes: []mongo.IndexModel{
				{Keys: bson.D{{"field_a", 1}, {"field_b", -1}}},
			},
		},
		{
			name: "unique_index",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{"unique_field", 1}},
					Options: options.Index().SetUnique(true),
				},
			},
		},
		{
			name: "sparse_index",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{"sparse_field", 1}},
					Options: options.Index().SetSparse(true),
				},
			},
		},
		{
			name: "ttl_index",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{"created_at", 1}},
					Options: options.Index().SetExpireAfterSeconds(86400),
				},
			},
		},
		{
			name: "text_index",
			indexes: []mongo.IndexModel{
				{Keys: bson.D{{"content", "text"}}},
			},
		},
		{
			name: "collation_collection",
			options: options.CreateCollection().SetCollation(&options.Collation{
				Locale:   "en",
				Strength: 2,
			}),
		},
		{
			name: "partial_filter_index",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{{"status", 1}},
					Options: options.Index().SetPartialFilterExpression(bson.D{
						{"status", bson.D{{"$eq", "active"}}},
					}),
				},
			},
		},
		{
			name: "timeseries_collection",
			timeseries: options.TimeSeries().
				SetTimeField("timestamp").
				SetMetaField("metadata"),
		},
	}
}

func (tc *testContext) createDumpDir() error {
	tempDir, err := os.MkdirTemp("", "rir-test-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	dumpDir := filepath.Join(tempDir, "dump")
	if err = os.MkdirAll(dumpDir, 0755); err != nil {
		return err
	}
	tc.dumpDir = dumpDir

	tc.barrierFile = filepath.Join(tempDir, "barrier-file")

	if tc.keepDump {
		log.Printf("Temp directory will be kept for inspection: %s", tempDir)
	} else {
		log.Printf("Using temp directory: %s", tempDir)
	}

	return nil
}

func (tc *testContext) removeDumpDir() {
	log.Printf("Removing dump directory: %s", tc.dumpDir)
	os.RemoveAll(tc.dumpDir)
}

func (tc *testContext) runDumpWithoutOplog() error {
	tempDir, err := os.MkdirTemp("", "rir-test-simple-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory for simple dump: %w", err)
	}
	tc.simpleDumpDir = filepath.Join(tempDir, "dump")
	if err = os.MkdirAll(tc.simpleDumpDir, 0755); err != nil {
		return fmt.Errorf("failed to create simple dump directory: %w", err)
	}
	if tc.keepDump {
		log.Printf("Simple dump temp directory will be kept for inspection: %s", tempDir)
	}

	log.Println("Starting mongodump without --oplog...")
	dumpCmd := exec.Command(
		"./bin/mongodump",
		"-vvvv",
		"--uri", tc.srcURI,
		"--out", tc.simpleDumpDir,
	)
	dumpCmd.Dir = repoRoot()
	dumpCmd.Stdout = os.Stdout
	dumpCmd.Stderr = os.Stderr
	if err := dumpCmd.Run(); err != nil {
		return fmt.Errorf("mongodump (no oplog) failed: %w", err)
	}
	log.Println("✅ mongodump (no oplog) completed")
	return nil
}

func (tc *testContext) removeSimpleDumpDir() {
	if tc.keepDump || tc.simpleDumpDir == "" {
		return
	}
	log.Printf("Removing simple dump directory: %s", tc.simpleDumpDir)
	os.RemoveAll(filepath.Dir(tc.simpleDumpDir))
}

func (tc *testContext) removeSystemDBsFromSimpleDump() error {
	for _, dbName := range []string{"admin", "config"} {
		p := filepath.Join(tc.simpleDumpDir, dbName)
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("failed to remove directory %#q: %w", p, err)
		}
	}
	log.Println("✅ removed system data directories from simple dump")
	return nil
}

func (tc *testContext) runRestoreWithoutOplog() error {
	log.Println("Running mongorestore without --oplogReplay --drop...")
	restoreCmd := exec.Command(
		"./bin/mongorestore",
		"-vvvv",
		"--uri", tc.dstURI,
		"--drop",
		tc.simpleDumpDir,
	)
	restoreCmd.Dir = repoRoot()
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr
	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("mongorestore (no oplog replay) failed: %w", err)
	}
	log.Println("✅ mongorestore (no oplog replay) completed")
	return nil
}

func (tc *testContext) runDumpWithConcurrentOps(ctx context.Context) error {
	log.Println("Starting mongodump with --oplog...")
	dumpCmd := exec.Command(
		"./bin/mongodump",
		"-vvvv",
		"--uri", tc.srcURI,
		"--oplog",
		"--out", tc.dumpDir,
		"--internalOnlySourceWritesDoneBarrier", tc.barrierFile,
	)
	dumpCmd.Dir = repoRoot()
	dumpCmd.Stdout = os.Stdout
	dumpCmd.Stderr = os.Stderr

	if err := dumpCmd.Start(); err != nil {
		return fmt.Errorf("failed to start mongodump: %w", err)
	}

	log.Printf("Waiting %v before starting concurrent operations...", dumpStartDelay)
	time.Sleep(dumpStartDelay)

	log.Println("Running concurrent operations...")
	if err := runConcurrentOperations(ctx, tc.srcClient, tc.barrierFile); err != nil {
		log.Printf("Warning: concurrent operations error: %v", err)
	}
	log.Println("✅ Concurrent operations completed")

	log.Println("Waiting for mongodump to complete...")
	if err := dumpCmd.Wait(); err != nil {
		return fmt.Errorf("mongodump failed: %w", err)
	}
	log.Println("✅ mongodump completed")

	return nil
}

// When we dump an entire cluster, `mongodump` will include the `config.settings` DB and users and
// roles from `admin`. When we try to restore this to an Atlas cluster, it blows up. We cannot use
// any sort of NS exclusion features with the oplog-related features, so we will just delete the
// directories with the dumped data for these DBs.
func (tc *testContext) removeSystemDBsFromDump() error {
	for _, dbName := range []string{"admin", "config"} {
		adminPath := filepath.Join(tc.dumpDir, dbName)
		if err := os.RemoveAll(adminPath); err != nil {
			return fmt.Errorf("failed to remove directory %#q: %w", adminPath, err)
		}
	}

	log.Println("✅ removed system data directories from dump")

	return nil
}

func (tc *testContext) verifyOplogHasEntries() error {
	log.Println("Verifying oplog file has entries...")
	oplogCount, err := countOplogEntries(tc.dumpDir)
	if err != nil {
		return fmt.Errorf("failed to count oplog entries: %w", err)
	}
	if oplogCount == 0 {
		return fmt.Errorf("oplog file has no entries - concurrent operations were not captured")
	}
	log.Printf("✅ Oplog file has %d entries", oplogCount)
	return nil
}

func countOplogEntries(dumpDir string) (int, error) {
	oplogPath := filepath.Join(dumpDir, "oplog.bson")

	file, err := os.Open(oplogPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open oplog file: %w", err)
	}
	defer file.Close()

	count := 0
	for {
		_, err := bson.ReadDocument(file)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read BSON document: %w", err)
		}
		count++
	}

	return count, nil
}

func (tc *testContext) runRestore() error {
	log.Println("Running mongorestore with --oplogReplay --drop...")
	restoreCmd := exec.Command(
		"./bin/mongorestore",
		"-vvvv",
		"--uri", tc.dstURI,
		"--oplogReplay",
		"--drop",
		tc.dumpDir,
	)
	restoreCmd.Dir = repoRoot()
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("mongorestore failed: %w", err)
	}
	log.Println("✅ mongorestore completed")
	return nil
}

func (tc *testContext) runRestoreExpectingFailure() error {
	log.Println(
		"Running mongorestore with --oplogReplay --drop (expecting failure due to recordIdsReplicated mismatch)...",
	)
	restoreCmd := exec.Command(
		"./bin/mongorestore",
		"-vvvv",
		"--uri", tc.dstURI,
		"--oplogReplay",
		"--drop",
		tc.dumpDir,
	)
	restoreCmd.Dir = repoRoot()
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err == nil {
		return fmt.Errorf(
			"mongorestore succeeded but should have failed: clusters differ in recordIdsReplicated support",
		)
	}
	log.Println("✅ mongorestore failed as expected")
	return nil
}

func (tc *testContext) verifyClustersMatch(ctx context.Context) error {
	log.Println("Verifying clusters match...")

	databasesToVerify := []string{testDBName, concurrentOpsDBName}

	for _, dbName := range databasesToVerify {
		log.Printf("  Verifying database: %s", dbName)

		srcDB := tc.srcClient.Database(dbName)
		dstDB := tc.dstClient.Database(dbName)

		srcColls, err := srcDB.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			return fmt.Errorf("failed to list source collections: %w", err)
		}

		dstColls, err := dstDB.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			return fmt.Errorf("failed to list destination collections: %w", err)
		}

		sort.Strings(srcColls)
		sort.Strings(dstColls)

		if len(srcColls) != len(dstColls) {
			return fmt.Errorf(
				"collection count mismatch in %s: source has %d, destination has %d\n  source: %v\n  dest: %v",
				dbName,
				len(srcColls),
				len(dstColls),
				srcColls,
				dstColls,
			)
		}

		for i, collName := range srcColls {
			if collName != dstColls[i] {
				return fmt.Errorf("collection name mismatch: source has %s, destination has %s",
					collName, dstColls[i])
			}
		}

		for _, collName := range srcColls {
			if strings.HasPrefix(collName, "system.") {
				continue
			}

			if err := verifyCollection(ctx, srcDB.Collection(collName), dstDB.Collection(collName), dbName, collName); err != nil {
				return err
			}
		}
	}

	log.Println("✅ Clusters match")
	return nil
}

func repoRoot() string {
	execPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(execPath, "build.go")); err == nil {
		return execPath
	}
	if _, err := os.Stat(filepath.Join(execPath, "../../build.go")); err == nil {
		return filepath.Join(execPath, "../..")
	}

	log.Fatal(
		"Cannot determine repo root - run from repo root or manual-tests/record-ids-replicated directory",
	)
	return ""
}

func insertTestDocuments(ctx context.Context, coll *mongo.Collection, spec collectionSpec) error {
	docs := make([]any, docsPerCollection)

	for i := 0; i < docsPerCollection; i++ {
		if spec.timeseries != nil {
			docs[i] = bson.D{
				{"timestamp", time.Now().Add(time.Duration(i) * time.Second)},
				{"metadata", bson.D{{"sensor", fmt.Sprintf("sensor_%d", i%5)}}},
				{"value", i * 10},
				{"reading", fmt.Sprintf("reading_%d", i)},
			}
		} else {
			docs[i] = bson.D{
				{"_id", bson.NewObjectID()},
				{"index", i},
				{"field_a", fmt.Sprintf("value_a_%d", i)},
				{"field_b", i * 2},
				{"unique_field", fmt.Sprintf("unique_%s_%d", spec.name, i)},
				{"sparse_field", func() any {
					if i%3 == 0 {
						return nil
					}
					return fmt.Sprintf("sparse_%d", i)
				}()},
				{"created_at", time.Now()},
				{"content", fmt.Sprintf("This is document number %d with some text content", i)},
				{"status", func() string {
					if i%2 == 0 {
						return "active"
					}
					return "inactive"
				}()},
			}
		}
	}

	_, err := coll.InsertMany(ctx, docs)
	return err
}

func runConcurrentOperations(ctx context.Context, client *mongo.Client, barrierFile string) error {
	log.Println("  Running concurrent operations for", concurrentOpsPeriod)

	db := client.Database(testDBName)
	concurrentDB := client.Database(concurrentOpsDBName)

	endTime := time.Now().Add(concurrentOpsPeriod)
	opCount := 0

	log.Println("  Creating new collection during dump...")
	if err := concurrentDB.CreateCollection(ctx, concurrentOpsColl); err != nil {
		return fmt.Errorf("failed to create concurrent collection: %w", err)
	}
	opCount++

	concurrentColl := concurrentDB.Collection(concurrentOpsColl)
	for i := 0; i < 20; i++ {
		_, err := concurrentColl.InsertOne(ctx, bson.D{
			{"_id", bson.NewObjectID()},
			{"concurrent_index", i},
			{"message", fmt.Sprintf("Concurrent insert %d", i)},
		})
		if err != nil {
			return fmt.Errorf("failed to insert into concurrent collection: %w", err)
		}
		opCount++
	}

	log.Println("  Creating index on concurrent collection...")
	_, err := concurrentColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{"concurrent_index", 1}},
	})
	if err != nil {
		return fmt.Errorf("failed to create index on concurrent collection: %w", err)
	}
	opCount++

	simpleColl := db.Collection("simple_collection")
	for time.Now().Before(endTime) {
		_, err := simpleColl.InsertOne(ctx, bson.D{
			{"_id", bson.NewObjectID()},
			{"concurrent", true},
			{"timestamp", time.Now()},
		})
		if err != nil {
			return fmt.Errorf("failed to insert during concurrent ops: %w", err)
		}
		opCount++

		_, err = simpleColl.UpdateOne(ctx,
			bson.D{{"index", 0}},
			bson.D{{"$set", bson.D{{"updated_during_dump", true}}}},
		)
		if err != nil {
			return fmt.Errorf("failed to update during concurrent ops: %w", err)
		}
		opCount++

		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("  Completed %d concurrent operations", opCount)

	if err = os.WriteFile(barrierFile, nil, 0644); err != nil {
		return fmt.Errorf("error writing to %s: %w", barrierFile, err)
	}

	log.Printf("  Created barrier file at %s", barrierFile)

	return nil
}

func verifyCollection(
	ctx context.Context,
	srcColl, dstColl *mongo.Collection,
	dbName, collName string,
) error {
	log.Printf("    Verifying collection: %s.%s", dbName, collName)

	srcCount, err := srcColl.CountDocuments(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("failed to count source documents in %s.%s: %w", dbName, collName, err)
	}

	dstCount, err := dstColl.CountDocuments(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf(
			"failed to count destination documents in %s.%s: %w",
			dbName,
			collName,
			err,
		)
	}

	if srcCount != dstCount {
		return fmt.Errorf(
			"document count mismatch in %s.%s: source has %d, destination has %d",
			dbName,
			collName,
			srcCount,
			dstCount,
		)
	}

	srcDocs, err := fetchAllDocuments(ctx, srcColl)
	if err != nil {
		return fmt.Errorf("failed to fetch source documents from %s.%s: %w", dbName, collName, err)
	}

	dstDocs, err := fetchAllDocuments(ctx, dstColl)
	if err != nil {
		return fmt.Errorf(
			"failed to fetch destination documents from %s.%s: %w",
			dbName,
			collName,
			err,
		)
	}

	for id, srcDoc := range srcDocs {
		dstDoc, exists := dstDocs[id]
		if !exists {
			return fmt.Errorf(
				"document with _id %#q exists in source but not in destination for %s.%s",
				id.String(),
				dbName,
				collName,
			)
		}

		srcBytes, _ := bson.Marshal(srcDoc)
		dstBytes, _ := bson.Marshal(dstDoc)

		if string(srcBytes) != string(dstBytes) {
			return fmt.Errorf(
				"document mismatch for _id %#q in %s.%s:\n  source: %v\n  dest: %v",
				id.String(),
				dbName,
				collName,
				srcDoc,
				dstDoc,
			)
		}
	}

	log.Printf("      ✅ %d documents match", srcCount)
	return nil
}

func fetchAllDocuments(
	ctx context.Context,
	coll *mongo.Collection,
) (map[bson.ObjectID]bson.D, error) {
	cursor, err := coll.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	docs := make(map[bson.ObjectID]bson.D)
	for cursor.Next(ctx) {
		var doc bson.D
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}

		var id bson.ObjectID
		for _, elem := range doc {
			if elem.Key == "_id" {
				var ok bool
				id, ok = elem.Value.(bson.ObjectID)
				if !ok {
					return nil, fmt.Errorf(
						"document has an _id field which is not an ObjectID - %+v",
						elem.Value,
					)
				}
				break
			}
		}

		docs[id] = doc
	}

	return docs, cursor.Err()
}
