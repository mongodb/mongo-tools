// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package mongorestore writes BSON data to a MongoDB instance.
package mongorestore

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mongodb/mongo-tools/common"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/auth"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongorestore/ns"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	progressBarLength                        = 24
	progressBarWaitTime                      = time.Second * 3
	deprecatedDBAndCollectionsOptionsWarning = "The --db and --collection flags are deprecated for " +
		"this use-case; please use --nsInclude instead, " +
		"i.e. with --nsInclude=${DATABASE}.${COLLECTION}"
)

var (
	NoUsersOrRolesInDumpError = errors.New(
		"No users or roles found in restore target. Please omit --restoreDbUsersAndRoles, or use a dump created with --dumpDbUsersAndRoles.",
	)
)

// MongoRestore is a container for the user-specified options and
// internal state used for running mongorestore.
type MongoRestore struct {
	ToolOptions   *options.ToolOptions
	InputOptions  *InputOptions
	OutputOptions *OutputOptions
	NSOptions     *NSOptions

	SessionProvider *db.SessionProvider
	ProgressManager progress.Manager

	TargetDirectory string

	// Skip restoring users and roles, regardless of namespace, when true.
	SkipUsersAndRoles bool

	// other internal state
	manager *intents.Manager

	objCheck     bool
	oplogLimit   bson.Timestamp
	isMongos     bool
	isAtlasProxy bool
	authVersions authVersionPair

	// a map of database names to a list of collection names
	knownCollections      map[string][]string
	knownCollectionsMutex sync.Mutex

	renamer  *ns.Renamer
	includer *ns.Matcher
	excluder *ns.Matcher

	// indexes belonging to dbs and collections
	dbCollectionIndexes map[string]collectionIndexes

	indexCatalog *idx.IndexCatalog

	archive *archive.Reader

	// boolean set if termination signal received; false by default
	terminate atomic.Bool

	// Reader to take care of BSON input if not reading from the local filesystem.
	// This is initialized to os.Stdin if unset.
	InputReader io.Reader

	// Server versions for version-specific behavior
	dumpServerVersion db.Version
	serverVersion     db.Version
}

type collectionIndexes map[string][]*idx.IndexDocument

// New initializes an instance of MongoRestore according to the provided options.
func New(opts Options) (*MongoRestore, error) {
	provider, err := db.NewSessionProvider(*opts.ToolOptions)
	if err != nil {
		return nil, fmt.Errorf("error connecting to host: %v", err)
	}

	serverVersion, err := provider.ServerVersionArray()
	if err != nil {
		return nil, fmt.Errorf("error getting server version: %v", err)
	}

	// start up the progress bar manager
	progressManager := progress.NewBarWriter(
		log.Writer(0),
		progressBarWaitTime,
		progressBarLength,
		true,
	)
	progressManager.Start()

	restore := &MongoRestore{
		ToolOptions:     opts.ToolOptions,
		OutputOptions:   opts.OutputOptions,
		InputOptions:    opts.InputOptions,
		NSOptions:       opts.NSOptions,
		TargetDirectory: opts.TargetDirectory,
		SessionProvider: provider,
		ProgressManager: progressManager,
		serverVersion:   serverVersion,
		indexCatalog:    idx.NewIndexCatalog(),
	}

	restore.isMongos, err = restore.SessionProvider.IsMongos()
	if err != nil {
		return nil, err
	}
	if restore.isMongos {
		log.Logv(log.DebugLow, "restoring to a sharded system")
	}
	restore.isAtlasProxy = restore.SessionProvider.IsAtlasProxy()
	if restore.isAtlasProxy {
		log.Logv(log.DebugLow, "restoring to a MongoDB Atlas free or shared cluster")
	}

	return restore, nil
}

// Close ends any connections and cleans up other internal state.
func (restore *MongoRestore) Close() {
	restore.SessionProvider.Close()
	barWriter, ok := restore.ProgressManager.(*progress.BarWriter)
	if ok { // should always be ok
		barWriter.Stop()
	}
}

// ParseAndValidateOptions returns a non-nil error if user-supplied options are invalid.
func (restore *MongoRestore) ParseAndValidateOptions() error {
	// Can't use option pkg defaults for --objcheck because it's two separate flags,
	// and we need to be able to see if they're both being used. We default to
	// true here and then see if noobjcheck is enabled.
	log.Logv(log.DebugHigh, "checking options")
	if restore.InputOptions.Objcheck {
		restore.objCheck = true
		log.Logv(log.DebugHigh, "\tdumping with object check enabled")
	} else {
		log.Logv(log.DebugHigh, "\tdumping with object check disabled")
	}

	if restore.ToolOptions.DB == "" && restore.ToolOptions.Collection != "" {
		return fmt.Errorf("cannot restore a collection without a specified database")
	}

	if restore.ToolOptions.DB != "" {
		if err := util.ValidateDBName(restore.ToolOptions.DB); err != nil {
			return fmt.Errorf("invalid db name: %v", err)
		}
	}
	if restore.ToolOptions.Collection != "" {
		if err := util.ValidateCollectionGrammar(restore.ToolOptions.Collection); err != nil {
			return fmt.Errorf("invalid collection name: %v", err)
		}
	}
	if restore.InputOptions.RestoreDBUsersAndRoles && restore.ToolOptions.DB == "" {
		return fmt.Errorf("cannot use --restoreDbUsersAndRoles without a specified database")
	}
	if restore.InputOptions.RestoreDBUsersAndRoles && restore.ToolOptions.DB == "admin" {
		return fmt.Errorf("cannot use --restoreDbUsersAndRoles with the admin database")
	}

	if restore.isAtlasProxy {
		if restore.InputOptions.RestoreDBUsersAndRoles ||
			restore.ToolOptions.DB == "admin" {
			return fmt.Errorf(
				"cannot restore to the admin database when connected to a MongoDB Atlas free or shared cluster",
			)
		}
		log.Logv(log.DebugLow, "restoring to a MongoDB Atlas free or shared cluster")
	}

	var err error
	if restore.InputOptions.OplogLimit != "" {
		if !restore.InputOptions.OplogReplay {
			return fmt.Errorf("cannot use --oplogLimit without --oplogReplay enabled")
		}
		restore.oplogLimit, err = ParseTimestampFlag(restore.InputOptions.OplogLimit)
		if err != nil {
			return fmt.Errorf("error parsing timestamp argument to --oplogLimit: %v", err)
		}
	}
	if restore.InputOptions.OplogFile != "" {
		if !restore.InputOptions.OplogReplay {
			return fmt.Errorf("cannot use --oplogFile without --oplogReplay enabled")
		}
		if restore.InputOptions.Archive != "" {
			return fmt.Errorf("cannot use --oplogFile with --archive specified")
		}
	}

	// check if we are using a replica set and fall back to w=1 if we aren't (for <= 2.4)
	nodeType, err := restore.SessionProvider.GetNodeType()
	if err != nil {
		return fmt.Errorf("error determining type of connected node: %v", err)
	}

	log.Logvf(log.DebugLow, "connected to node type: %v", nodeType)

	// deprecations with --nsInclude --nsExclude
	if restore.ToolOptions.DB != "" || restore.ToolOptions.Collection != "" {
		if filepath.Ext(restore.TargetDirectory) != ".bson" {
			log.Logvf(log.Always, deprecatedDBAndCollectionsOptionsWarning)
		}
	}
	if len(restore.NSOptions.ExcludedCollections) > 0 ||
		len(restore.NSOptions.ExcludedCollectionPrefixes) > 0 {
		log.Logvf(log.Always, "the --excludeCollections and --excludeCollectionPrefixes options "+
			"are deprecated and will not exist in the future; use --nsExclude instead")
	}
	if restore.InputOptions.OplogReplay {
		if len(restore.NSOptions.NSInclude) > 0 || restore.ToolOptions.DB != "" {
			return fmt.Errorf("cannot use --oplogReplay with includes specified")
		}
		if len(restore.NSOptions.NSExclude) > 0 || len(restore.NSOptions.ExcludedCollections) > 0 ||
			len(restore.NSOptions.ExcludedCollectionPrefixes) > 0 {
			return fmt.Errorf("cannot use --oplogReplay with excludes specified")
		}
		if len(restore.NSOptions.NSFrom) > 0 {
			return fmt.Errorf("cannot use --oplogReplay with namespace renames specified")
		}
	}

	includes := restore.NSOptions.NSInclude
	if restore.ToolOptions.DB != "" && restore.ToolOptions.Collection != "" {
		includes = append(includes, ns.Escape(restore.ToolOptions.DB)+"."+
			restore.ToolOptions.Collection)
	} else if restore.ToolOptions.DB != "" {
		includes = append(includes, ns.Escape(restore.ToolOptions.DB)+".*")
	}
	if len(includes) == 0 {
		includes = []string{"*"}
	}
	restore.includer, err = ns.NewMatcher(includes)
	if err != nil {
		return fmt.Errorf("invalid includes: %v", err)
	}

	if len(restore.NSOptions.ExcludedCollections) > 0 &&
		restore.ToolOptions.Collection != "" {
		return fmt.Errorf("--collection is not allowed when --excludeCollection is specified")
	}
	if len(restore.NSOptions.ExcludedCollectionPrefixes) > 0 &&
		restore.ToolOptions.Collection != "" {
		return fmt.Errorf(
			"--collection is not allowed when --excludeCollectionsWithPrefix is specified",
		)
	}
	excludes := restore.NSOptions.NSExclude
	for _, col := range restore.NSOptions.ExcludedCollections {
		excludes = append(excludes, "*."+ns.Escape(col))
	}
	for _, colPrefix := range restore.NSOptions.ExcludedCollectionPrefixes {
		excludes = append(excludes, "*."+ns.Escape(colPrefix)+"*")
	}
	restore.excluder, err = ns.NewMatcher(excludes)
	if err != nil {
		return fmt.Errorf("invalid excludes: %v", err)
	}

	if len(restore.NSOptions.NSFrom) != len(restore.NSOptions.NSTo) {
		return fmt.Errorf(
			"--nsFrom and --nsTo arguments must be specified an equal number of times",
		)
	}
	restore.renamer, err = ns.NewRenamer(restore.NSOptions.NSFrom, restore.NSOptions.NSTo)
	if err != nil {
		return fmt.Errorf("invalid renames: %v", err)
	}

	if restore.OutputOptions.NumInsertionWorkers < 0 {
		return fmt.Errorf(
			"cannot specify a negative number of insertion workers per collection")
	}

	if restore.OutputOptions.MaintainInsertionOrder {
		restore.OutputOptions.StopOnError = true
		restore.OutputOptions.NumInsertionWorkers = 1
	}

	if restore.OutputOptions.PreserveUUID && !restore.OutputOptions.Drop {
		return fmt.Errorf("cannot specify --preserveUUID without --drop")
	}

	// a single dash signals reading from stdin
	if restore.TargetDirectory == "-" {
		if restore.InputOptions.Archive != "" {
			return fmt.Errorf(
				"cannot restore from \"-\" when --archive is specified")
		}
		if restore.ToolOptions.Collection == "" {
			return fmt.Errorf("cannot restore from stdin without a specified collection")
		}
	}
	if restore.InputReader == nil {
		restore.InputReader = os.Stdin
	}

	return nil
}

// Restore runs the mongorestore program.
func (restore *MongoRestore) Restore() Result {
	var target archive.DirLike
	err := restore.ParseAndValidateOptions()
	if err != nil {
		log.Logvf(log.DebugLow, "got error from options parsing: %v", err)
		return Result{Err: err}
	}

	// Build up all intents to be restored
	restore.manager = intents.NewIntentManager()
	if restore.InputOptions.Archive == "" && restore.InputOptions.OplogReplay {
		restore.manager.SetSmartPickOplog(true)
	}

	if restore.InputOptions.Archive != "" {
		if restore.archive == nil {
			archiveReader, err := restore.getArchiveReader()
			if err != nil {
				return Result{Err: err}
			}
			restore.archive = &archive.Reader{
				In:      archiveReader,
				Prelude: &archive.Prelude{},
			}
			defer restore.archive.In.Close()
		}
		err = restore.archive.Prelude.Read(restore.archive.In)
		if err != nil {
			return Result{Err: err}
		}
		log.Logvf(
			log.DebugLow,
			`archive format version "%v"`,
			restore.archive.Prelude.Header.FormatVersion,
		)

		dumpServerVersionStr := restore.archive.Prelude.Header.ServerVersion
		log.Logvf(
			log.DebugLow,
			`archive server version "%v"`,
			dumpServerVersionStr,
		)
		restore.dumpServerVersion, _ = db.StrToVersion(dumpServerVersionStr)
		log.Logvf(
			log.DebugLow,
			`archive tool version "%v"`,
			restore.archive.Prelude.Header.ToolVersion,
		)

		if restore.dumpServerVersion.CmpMinor(restore.serverVersion) != 0 {
			log.Logvf(
				log.Always,
				"WARNING: This archive came from MongoDB %s, but you are restoring to %s. Cross-version dump & restore is unsupported. The restored data may be corrupted.",
				dumpServerVersionStr,
				restore.serverVersion.String(),
			)
		}

		target, err = restore.archive.Prelude.NewPreludeExplorer()
		if err != nil {
			return Result{Err: err}
		}
	} else if restore.TargetDirectory != "-" {
		var usedDefaultTarget bool
		if restore.TargetDirectory == "" {
			restore.TargetDirectory = "dump"
			log.Logv(log.Always, "using default 'dump' directory")
			usedDefaultTarget = true
		}
		target, err = newActualPath(restore.TargetDirectory)
		if err != nil {
			if usedDefaultTarget {
				log.Logv(log.Always, util.ShortUsage("mongorestore"))
			}
			return Result{Err: fmt.Errorf("mongorestore target '%v' invalid: %v", restore.TargetDirectory, err)}
		}
		preludeFileExists, err := restore.ReadPreludeMetadata(target)
		if !preludeFileExists {
			// don't error out here because mongodump versions before 100.12.0 will not include prelude.json
			log.Logvf(log.DebugLow, "no prelude metadata found in target directory or parent, skipping")
		} else if err != nil {
			return Result{Err: fmt.Errorf("error reading dump metadata: %w", err)}
		}

		// handle cases where the user passes in a file instead of a directory
		if !target.IsDir() {
			log.Logv(log.DebugLow, "mongorestore target is a file, not a directory")
			err = restore.handleBSONInsteadOfDirectory(restore.TargetDirectory)
			if err != nil {
				return Result{Err: err}
			}
		} else {
			log.Logv(log.DebugLow, "mongorestore target is a directory, not a file")
		}
	}
	if restore.ToolOptions.Collection != "" &&
		restore.OutputOptions.NumParallelCollections > 1 &&
		restore.OutputOptions.NumInsertionWorkers == 1 &&
		!restore.OutputOptions.MaintainInsertionOrder {
		// handle special parallelization case when we are only restoring one collection
		// by mapping -j to insertion workers rather than parallel collections
		log.Logvf(log.DebugHigh,
			"setting number of insertions workers to number of parallel collections (%v)",
			restore.OutputOptions.NumParallelCollections)
		restore.OutputOptions.NumInsertionWorkers = restore.OutputOptions.NumParallelCollections
	}
	if restore.InputOptions.Archive != "" {
		if int(
			restore.archive.Prelude.Header.ConcurrentCollections,
		) > restore.OutputOptions.NumParallelCollections {
			restore.OutputOptions.NumParallelCollections = int(
				restore.archive.Prelude.Header.ConcurrentCollections,
			)
			log.Logvf(
				log.Always,
				"setting number of parallel collections to number of parallel collections in archive (%v)",
				restore.archive.Prelude.Header.ConcurrentCollections,
			)
		}
	}

	// Create the demux before intent creation, because muted archive intents need
	// to register themselves with the demux directly
	if restore.InputOptions.Archive != "" {
		restore.archive.Demux = archive.CreateDemux(
			restore.archive.Prelude.NamespaceMetadatas,
			restore.archive.In,
			restore.isAtlasProxy,
		)
	}

	switch {
	case restore.InputOptions.Archive != "":
		log.Logvf(log.Always, "preparing collections to restore from")
		err = restore.CreateAllIntents(target)
	case restore.ToolOptions.DB != "" && restore.ToolOptions.Collection == "":
		log.Logvf(log.Always,
			"building a list of collections to restore from %v dir",
			target.Path())
		err = restore.CreateIntentsForDB(
			restore.ToolOptions.DB,
			target,
		)
	case restore.ToolOptions.DB != "" && restore.ToolOptions.Collection != "" && restore.TargetDirectory == "-":
		log.Logvf(log.Always, "setting up a collection to be read from standard input")
		err = restore.CreateStdinIntentForCollection(
			restore.ToolOptions.DB,
			restore.ToolOptions.Collection,
		)
	case restore.ToolOptions.DB != "" && restore.ToolOptions.Collection != "":
		log.Logvf(log.Always, "checking for collection data in %v", target.Path())
		err = restore.CreateIntentForCollection(
			restore.ToolOptions.DB,
			restore.ToolOptions.Collection,
			target,
		)
	default:
		log.Logvf(log.Always, "preparing collections to restore from")
		err = restore.CreateAllIntents(target)
	}
	if err != nil {
		return Result{Err: fmt.Errorf("error scanning filesystem: %v", err)}
	}

	if restore.isMongos && restore.manager.HasConfigDBIntent() &&
		restore.ToolOptions.DB == "" {
		return Result{Err: fmt.Errorf("cannot do a full restore on a sharded system - " +
			"remove the 'config' directory from the dump directory first")}
	}

	// if --restoreDbUsersAndRoles is used then db specific users and roles ($admin.system.users.bson / $admin.system.roles.bson)
	// should exist in target directory / archive
	if restore.InputOptions.RestoreDBUsersAndRoles &&
		restore.ToolOptions.DB != "" &&
		(restore.manager.Users() == nil && restore.manager.Roles() == nil) {
		return Result{
			Err: NoUsersOrRolesInDumpError,
		}
	}

	if restore.InputOptions.OplogFile != "" {
		err = restore.CreateIntentForOplog()
		if err != nil {
			return Result{Err: fmt.Errorf("error reading oplog file: %v", err)}
		}
	}
	if restore.InputOptions.OplogReplay && restore.manager.Oplog() == nil {
		return Result{
			Err: fmt.Errorf("no oplog file to replay; make sure you run mongodump with --oplog"),
		}
	}
	if restore.manager.GetOplogConflict() {
		return Result{
			Err: fmt.Errorf(
				"cannot provide both an oplog.bson file and an oplog file with --oplogFile, " +
					"nor can you provide both a local/oplog.rs.bson and a local/oplog.$main.bson file",
			),
		}
	}

	conflicts := restore.manager.GetDestinationConflicts()
	if len(conflicts) > 0 {
		for _, conflict := range conflicts {
			log.Logvf(log.Always, "%s", conflict.Error())
		}
		return Result{Err: fmt.Errorf("cannot restore with conflicting namespace destinations")}
	}

	if restore.OutputOptions.DryRun {
		log.Logvf(log.Always, "dry run completed")
		return Result{}
	}

	demuxFinished := make(chan interface{})
	var demuxErr error
	if restore.InputOptions.Archive != "" {
		namespaceChan := make(chan string, 1)
		namespaceErrorChan := make(chan error)
		restore.archive.Demux.NamespaceChan = namespaceChan
		restore.archive.Demux.NamespaceErrorChan = namespaceErrorChan

		go func() {
			demuxErr = restore.archive.Demux.Run()
			close(demuxFinished)
		}()
		// consume the new namespace announcement from the demux for all of the special collections
		// that get cached when being read out of the archive.
		// The first regular collection found gets pushed back on to the namespaceChan
		// consume the new namespace announcement from the demux for all of the collections that get cached
		for {
			ns, ok := <-namespaceChan
			// the archive can have only special collections. In that case we keep reading until
			// the namespaces are exhausted, indicated by the namespaceChan being closed.
			log.Logvf(log.DebugLow, "received %v from namespaceChan", ns)
			if !ok {
				break
			}
			dbName, collName := util.SplitNamespace(ns)
			ns = dbName + "." + strings.TrimPrefix(collName, "system.buckets.")
			intent := restore.manager.IntentForNamespace(ns)
			if intent == nil {
				return Result{Err: fmt.Errorf("no intent for collection in archive: %v", ns)}
			}
			if intent.IsSystemIndexes() ||
				intent.IsUsers() ||
				intent.IsRoles() ||
				intent.IsAuthVersion() {
				log.Logvf(log.DebugLow, "special collection %v found", ns)
				namespaceErrorChan <- nil
			} else {
				// Put the ns back on the announcement chan so that the
				// demultiplexer can start correctly
				log.Logvf(log.DebugLow, "first non special collection %v found."+
					" The demultiplexer will handle it and the remainder", ns)
				namespaceChan <- ns
				break
			}
		}
	}

	// If restoring users and roles, make sure we validate auth versions
	if restore.ShouldRestoreUsersAndRoles() {
		log.Logv(log.Info, "comparing auth version of the dump directory and target server")
		restore.authVersions.Dump, err = restore.GetDumpAuthVersion()
		if err != nil {
			return Result{Err: fmt.Errorf("error getting auth version from dump: %v", err)}
		}
		restore.authVersions.Server, err = auth.GetAuthVersion(restore.SessionProvider)
		if err != nil {
			return Result{Err: fmt.Errorf("error getting auth version of server: %v", err)}
		}
		err = restore.ValidateAuthVersions()
		if err != nil {
			return Result{Err: fmt.Errorf(
				"the users and roles collections in the dump have an incompatible auth version with target server: %v",
				err,
			)}
		}
	}

	err = restore.LoadIndexesFromBSON()
	if err != nil {
		return Result{Err: fmt.Errorf("restore error: %v", err)}
	}

	err = restore.PopulateMetadataForIntents()
	if err != nil {
		return Result{Err: fmt.Errorf("restore error: %v", err)}
	}

	err = restore.preFlightChecks()
	if err != nil {
		return Result{Err: fmt.Errorf("restore error: %v", err)}
	}

	// Restore the regular collections
	if restore.InputOptions.Archive != "" {
		restore.manager.UsePrioritizer(restore.archive.Demux.NewPrioritizer(restore.manager))
	} else if restore.OutputOptions.NumParallelCollections > 1 {
		// 3.0+ has collection-level locking for writes, so it is most efficient to
		// prioritize by collection size. Pre-3.0 we try to avoid inserting into collections
		// in the same database simultaneously due to the database-level locking.
		// Up to 4.2, foreground index builds take a database-level lock for the entire build,
		// but this prioritizer is not used for index builds so we don't need to worry about that here.
		if restore.serverVersion.GTE(db.Version{3, 0, 0}) {
			restore.manager.Finalize(intents.LongestTaskFirst)
		} else {
			restore.manager.Finalize(intents.MultiDatabaseLTF)
		}
	} else {
		// use legacy restoration order if we are single-threaded
		restore.manager.Finalize(intents.Legacy)
	}

	result := restore.RestoreIntents()
	if result.Err != nil {
		return result
	}

	// Restore users/roles
	if restore.ShouldRestoreUsersAndRoles() {
		err = restore.RestoreUsersOrRoles(restore.manager.Users(), restore.manager.Roles())
		if err != nil {
			return result.withErr(fmt.Errorf("restore error: %v", err))
		}
	}

	// Restore oplog
	if restore.InputOptions.OplogReplay {
		err = restore.RestoreOplog()
		if err != nil {
			return result.withErr(fmt.Errorf("restore error: %v", err))
		}
	}

	if !restore.OutputOptions.NoIndexRestore {
		err = restore.RestoreIndexes()
		if err != nil {
			return result.withErr(err)
		}
	}

	if restore.InputOptions.Archive != "" {
		<-demuxFinished
		return result.withErr(demuxErr)
	}

	return result
}

// ReadPreludeMetadata finds and parses the prelude.json file if it's present.
// It currently only sets the server.dumpServerVersion, but in the future we can read and set other metadata from the dump as required.
// Returns true if the metadata file exists.
func (restore *MongoRestore) ReadPreludeMetadata(target archive.DirLike) (bool, error) {
	filename := "prelude.json"
	if restore.InputOptions.Gzip {
		filename += ".gz"
	}

	var err error
	var reader io.ReadCloser
	if !target.IsDir() {
		// Look for prelude.json in target's directory if target is .bson file.
		target, err = newActualPath(target.Parent().Path())
		if err != nil {
			return false, fmt.Errorf("error finding parent of target file: %w", err)
		}
	}
	filePath := filepath.Join(target.Path(), filename)
	file, err := os.Open(filePath)
	if errors.Is(err, os.ErrNotExist) {
		// If the mongodump was for all databases, prelude.json will be in the top level directory.
		// If a single database's directory was used as the target, look for prelude.json in the target's parent directory.
		filePath = filepath.Join(target.Parent().Path(), filename)
		file, err = os.Open(filePath)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, fmt.Errorf("error opening file %#q: %w", filePath, err)
		}
	} else if err != nil {
		return false, fmt.Errorf("error opening file %#q: %w", filePath, err)
	}

	defer file.Close()

	if restore.InputOptions.Gzip {
		zipfile, err := gzip.NewReader(file)
		if err != nil {
			return true, fmt.Errorf("failed to open gzip file %#q: %w", filePath, err)
		}
		defer zipfile.Close()
		reader = zipfile
	} else {
		reader = file
	}
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return true, fmt.Errorf("failed to read prelude metadata from %#q: %w", filePath, err)
	}

	var prelude map[string]string
	err = json.Unmarshal(bytes, &prelude)
	if err != nil {
		return true, fmt.Errorf("failed to unmarshal prelude metadata from %#q: %w", filePath, err)
	}

	dumpVersion, ok := prelude["ServerVersion"]
	if !ok {
		return true, fmt.Errorf("ServerVersion key not found in %#q", filePath)
	}

	// mongodump sets server version to unknown if it can't get the server version
	if dumpVersion == common.ServerVersionUnknown {
		log.Logvf(log.Info, "ServerVersion is 'unknown' in %#q", filePath)
		return true, nil
	}

	restore.dumpServerVersion, err = db.StrToVersion(dumpVersion)
	if err != nil {
		return true, fmt.Errorf("failed to parse server version from %#q: %w", filePath, err)
	} else {
		log.Logvf(log.Info, "successfully parsed prelude metadata from %#q", filePath)
		log.Logvf(log.DebugLow, "restore.dumpServerVersion: %#q", dumpVersion)
		return true, nil
	}
}

func (restore *MongoRestore) preFlightChecks() error {

	for _, intent := range restore.manager.Intents() {
		if intent.Type == "timeseries" {

			if !restore.OutputOptions.Drop {
				timeseriesExists, err := restore.CollectionExists(intent.DB, intent.C)
				if err != nil {
					return err
				}

				if timeseriesExists {
					return fmt.Errorf(
						"timeseries collection `%s` already exists on the destination. "+
							"You must remove this collection from the destination or use --drop",
						intent.Namespace(),
					)
				}

				bucketExists, err := restore.CollectionExists(intent.DB, intent.DataCollection())
				if err != nil {
					return err
				}

				if bucketExists {
					return fmt.Errorf(
						"system.buckets collection `%v` already exists on the destination. "+
							"You must remove this collection from the destination in order to restore %s",
						intent.DataNamespace(),
						intent.Namespace(),
					)
				}
			}

			if restore.OutputOptions.NoOptionsRestore {
				return fmt.Errorf(
					"cannot specify --noOptionsRestore when restoring timeseries collections",
				)
			}
		}
	}

	if restore.serverVersion.GTE(db.Version{4, 9, 0}) && !restore.OutputOptions.NoIndexRestore {
		namespaces := restore.indexCatalog.Namespaces()
		for _, ns := range namespaces {
			indexes := restore.indexCatalog.GetIndexes(ns.DB, ns.Collection)
			for _, index := range indexes {
				for _, keyElement := range index.Key {
					if keyElement.Value == "geoHaystack" {
						return fmt.Errorf("found a geoHaystack index: %v on %s. "+
							"geoHaystack indexes are not supported by the destination cluster. "+
							"Remove the index from the source or use --noIndexRestore to skip all indexes.", index.Key, ns.String())
					}
				}
			}
		}
	}

	return nil
}

func (restore *MongoRestore) getArchiveReader() (rc io.ReadCloser, err error) {
	if restore.InputOptions.Archive == "-" {
		rc = io.NopCloser(restore.InputReader)
	} else {
		targetStat, err := os.Stat(restore.InputOptions.Archive)
		if err != nil {
			return nil, err
		}
		if targetStat.IsDir() {
			defaultArchiveFilePath := filepath.Join(restore.InputOptions.Archive, "archive")
			if restore.InputOptions.Gzip {
				defaultArchiveFilePath = defaultArchiveFilePath + ".gz"
			}
			rc, err = os.Open(defaultArchiveFilePath)
			if err != nil {
				return nil, err
			}
		} else {
			rc, err = os.Open(restore.InputOptions.Archive)
			if err != nil {
				return nil, err
			}
		}
	}
	if restore.InputOptions.Gzip {
		gzrc, err := gzip.NewReader(rc)
		if err != nil {
			return nil, err
		}
		return &util.WrappedReadCloser{gzrc, rc}, nil
	}
	return rc, nil
}

func (restore *MongoRestore) HandleInterrupt() {
	restore.terminate.Store(true)
}

func (restore *MongoRestore) writeContext() (context.Context, context.CancelFunc) {
	if wtimeout := restore.ToolOptions.WriteConcern.WTimeout; wtimeout > 0 {
		return context.WithTimeout(context.TODO(), wtimeout)
	}

	return context.WithCancel(context.TODO())
}
