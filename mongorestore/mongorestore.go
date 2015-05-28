// Package mongorestore writes BSON data to a MongoDB instance.
package mongorestore

import (
	"compress/gzip"
	"fmt"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/auth"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// MongoRestore is a container for the user-specified options and
// internal state used for running mongorestore.
type MongoRestore struct {
	ToolOptions   *options.ToolOptions
	InputOptions  *InputOptions
	OutputOptions *OutputOptions

	SessionProvider *db.SessionProvider

	TargetDirectory string

	tempUsersCol string
	tempRolesCol string

	// other internal state
	manager         *intents.Manager
	safety          *mgo.Safe
	progressManager *progress.Manager

	objCheck         bool
	oplogLimit       bson.MongoTimestamp
	useStdin         bool
	isMongos         bool
	useWriteCommands bool
	authVersions     authVersionPair

	// a map of database names to a list of collection names
	knownCollections      map[string][]string
	knownCollectionsMutex sync.Mutex

	// indexes belonging to dbs and collections
	dbCollectionIndexes map[string]collectionIndexes

	archive *archive.Reader
}

type collectionIndexes map[string][]IndexDocument

// ParseAndValidateOptions returns a non-nil error if user-supplied options are invalid.
func (restore *MongoRestore) ParseAndValidateOptions() error {
	// Can't use option pkg defaults for --objcheck because it's two separate flags,
	// and we need to be able to see if they're both being used. We default to
	// true here and then see if noobjcheck is enabled.
	log.Log(log.DebugHigh, "checking options")
	if restore.InputOptions.Objcheck {
		restore.objCheck = true
		log.Log(log.DebugHigh, "\tdumping with object check enabled")
	} else {
		log.Log(log.DebugHigh, "\tdumping with object check disabled")
	}

	if restore.ToolOptions.DB == "" && restore.ToolOptions.Collection != "" {
		return fmt.Errorf("cannot dump a collection without a specified database")
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

	var err error
	restore.isMongos, err = restore.SessionProvider.IsMongos()
	if err != nil {
		return err
	}
	if restore.isMongos {
		log.Log(log.DebugLow, "restoring to a sharded system")
	}

	if restore.InputOptions.OplogLimit != "" {
		if !restore.InputOptions.OplogReplay {
			return fmt.Errorf("cannot use --oplogLimit without --oplogReplay enabled")
		}
		restore.oplogLimit, err = ParseTimestampFlag(restore.InputOptions.OplogLimit)
		if err != nil {
			return fmt.Errorf("error parsing timestamp argument to --oplogLimit: %v", err)
		}
	}

	// check if we are using a replica set and fall back to w=1 if we aren't (for <= 2.4)
	nodeType, err := restore.SessionProvider.GetNodeType()
	if err != nil {
		return fmt.Errorf("error determining type of connected node: %v", err)
	}

	log.Logf(log.DebugLow, "connected to node type: %v", nodeType)
	restore.safety, err = db.BuildWriteConcern(restore.OutputOptions.WriteConcern, nodeType)
	if err != nil {
		return fmt.Errorf("error parsing write concern: %v", err)
	}

	// handle the hidden auth collection flags
	if restore.ToolOptions.HiddenOptions.TempUsersColl == nil {
		restore.tempUsersCol = "tempusers"
	} else {
		restore.tempUsersCol = *restore.ToolOptions.HiddenOptions.TempUsersColl
	}
	if restore.ToolOptions.HiddenOptions.TempRolesColl == nil {
		restore.tempRolesCol = "temproles"
	} else {
		restore.tempRolesCol = *restore.ToolOptions.HiddenOptions.TempRolesColl
	}

	if restore.OutputOptions.NumInsertionWorkers < 0 {
		return fmt.Errorf(
			"cannot specify a negative number of insertion workers per collection")
	}

	// a single dash signals reading from stdin
	if restore.TargetDirectory == "-" {
		restore.useStdin = true
		if restore.InputOptions.Archive != "" {
			return fmt.Errorf(
				"cannot restore from \"-\" when --archive is specified")
		}
		if restore.ToolOptions.Collection == "" {
			return fmt.Errorf("cannot restore from stdin without a specified collection")
		}
	}

	return nil
}

// Restore runs the mongorestore program.
func (restore *MongoRestore) Restore() error {
	var target archive.DirLike
	err := restore.ParseAndValidateOptions()
	if err != nil {
		log.Logf(log.DebugLow, "got error from options parsing: %v", err)
		return err
	}

	// Build up all intents to be restored
	restore.manager = intents.NewIntentManager()

	if restore.InputOptions.Archive != "" {
		archiveReader, err := restore.getArchiveReader()
		if err != nil {
			return err
		}
		restore.archive = &archive.Reader{
			In:      archiveReader,
			Prelude: &archive.Prelude{},
		}
		err = restore.archive.Prelude.Read(restore.archive.In)
		if err != nil {
			return err
		}
		target, err = restore.archive.Prelude.NewPreludeExplorer()
		if err != nil {
			return err
		}
	} else {
		if restore.TargetDirectory == "" {
			restore.TargetDirectory = "dump"
			log.Log(log.Always, "using default 'dump' directory")
		}
		target, err = newActualPath(restore.TargetDirectory)
		if err != nil {
			return err
		}
		// handle cases where the user passes in a file instead of a directory
		if !target.IsDir() {
			log.Log(log.DebugLow, "mongorestore target is a file, not a directory")
			err = restore.handleBSONInsteadOfDirectory(restore.TargetDirectory)
			if err != nil {
				return err
			}
		} else {
			log.Log(log.DebugLow, "mongorestore target is a directory, not a file")
		}
	}
	if restore.ToolOptions.Collection != "" &&
		restore.OutputOptions.NumParallelCollections > 1 &&
		restore.OutputOptions.NumInsertionWorkers == 1 {
		// handle special parallelization case when we are only restoring one collection
		// by mapping -j to insertion workers rather than parallel collections
		log.Logf(log.DebugHigh,
			"setting number of insertions workers to number of parallel collections (%v)",
			restore.OutputOptions.NumParallelCollections)
		restore.OutputOptions.NumInsertionWorkers = restore.OutputOptions.NumParallelCollections
	}
	if restore.InputOptions.Archive != "" {
		if int(restore.archive.Prelude.Header.ConcurrentCollections) > restore.OutputOptions.NumParallelCollections {
			restore.OutputOptions.NumParallelCollections = int(restore.archive.Prelude.Header.ConcurrentCollections)
			restore.OutputOptions.NumInsertionWorkers = int(restore.archive.Prelude.Header.ConcurrentCollections)
			log.Logf(log.Always,
				"setting number of parallel collections to number of parallel collections in archive (%v)",
				restore.archive.Prelude.Header.ConcurrentCollections,
			)
		}
	}

	// Create the demux before intent creation, because muted archive intents need
	// to register themselves with the demux directly
	if restore.InputOptions.Archive != "" {
		restore.archive.Demux = &archive.Demultiplexer{
			In: restore.archive.In,
		}
	}

	switch {
	case restore.InputOptions.Archive != "":
		log.Logf(log.Always,
			"creating intents for archive")
		err = restore.CreateAllIntents(target, restore.ToolOptions.DB, restore.ToolOptions.Collection)
	case restore.ToolOptions.DB == "" && restore.ToolOptions.Collection == "":
		log.Logf(log.Always,
			"building a list of dbs and collections to restore from %v dir",
			target.Path())
		err = restore.CreateAllIntents(target, "", "")
	case restore.ToolOptions.DB != "" && restore.ToolOptions.Collection == "":
		log.Logf(log.Always,
			"building a list of collections to restore from %v dir",
			target.Path())
		err = restore.CreateIntentsForDB(
			restore.ToolOptions.DB,
			"",
			target,
			false,
		)
	case restore.ToolOptions.DB != "" && restore.ToolOptions.Collection != "":
		log.Logf(log.Always, "checking for collection data in %v", target.Path())
		err = restore.CreateIntentForCollection(
			restore.ToolOptions.DB,
			restore.ToolOptions.Collection,
			target,
		)
	}
	if err != nil {
		return fmt.Errorf("error scanning filesystem: %v", err)
	}

	if restore.isMongos && restore.manager.HasConfigDBIntent() && restore.ToolOptions.DB == "" {
		return fmt.Errorf("cannot do a full restore on a sharded system - " +
			"remove the 'config' directory from the dump directory first")
	}

	if restore.InputOptions.Archive != "" {
		namespaceChan := make(chan string, 1)
		namespaceErrorChan := make(chan error)
		restore.archive.Demux.NamespaceChan = namespaceChan
		restore.archive.Demux.NamespaceErrorChan = namespaceErrorChan

		go restore.archive.Demux.Run()
		// consume the new namespace announcement from the demux for all of the collections that get cached
		for {
			ns, ok := <-namespaceChan
			// the archive can have only special collections
			if !ok {
				break
			}
			intent := restore.manager.IntentForNamespace(ns)
			if intent == nil {
				return fmt.Errorf("no intent for collection in archive: %v", ns)
			}
			if intent.IsSystemIndexes() ||
				intent.IsUsers() ||
				intent.IsRoles() ||
				intent.IsAuthVersion() {
				log.Logf(log.DebugLow, "special collection %v found", ns)
				namespaceErrorChan <- nil
			} else {
				// Put the ns back on the announcement chan so that the
				// demultiplexer can start correctly
				log.Logf(log.DebugLow, "first non special collection %v found."+
					" The demultiplexer will handle it and the remainder", ns)
				namespaceChan <- ns
				break
			}
		}
	}

	// If restoring users and roles, make sure we validate auth versions
	if restore.ShouldRestoreUsersAndRoles() {
		log.Log(log.Info, "comparing auth version of the dump directory and target server")
		restore.authVersions.Dump, err = restore.GetDumpAuthVersion()
		if err != nil {
			return fmt.Errorf("error getting auth version from dump: %v", err)
		}
		restore.authVersions.Server, err = auth.GetAuthVersion(restore.SessionProvider)
		if err != nil {
			return fmt.Errorf("error getting auth version of server: %v", err)
		}
		err = restore.ValidateAuthVersions()
		if err != nil {
			return fmt.Errorf(
				"the users and roles collections in the dump have an incompatible auth version with target server: %v",
				err)
		}
	}

	err = restore.LoadIndexesFromBSON()
	if err != nil {
		return fmt.Errorf("restore error: %v", err)
	}

	// Restore the regular collections
	if restore.InputOptions.Archive != "" {
		restore.manager.UsePrioritizer(restore.archive.Demux.NewPrioritizer(restore.manager))
	} else if restore.OutputOptions.NumParallelCollections > 1 {
		restore.manager.Finalize(intents.MultiDatabaseLTF)
	} else {
		// use legacy restoration order if we are single-threaded
		restore.manager.Finalize(intents.Legacy)
	}

	err = restore.RestoreIntents()
	if err != nil {
		return fmt.Errorf("restore error: %v", err)
	}

	// Restore users/roles
	if restore.ShouldRestoreUsersAndRoles() {
		if restore.manager.Users() != nil {
			err = restore.RestoreUsersOrRoles(Users, restore.manager.Users())
			if err != nil {
				return fmt.Errorf("restore error: %v", err)
			}
		}
		if restore.manager.Roles() != nil {
			err = restore.RestoreUsersOrRoles(Roles, restore.manager.Roles())
			if err != nil {
				return fmt.Errorf("restore error: %v", err)
			}
		}
	}

	// Restore oplog
	if restore.InputOptions.OplogReplay {
		err = restore.RestoreOplog()
		if err != nil {
			return fmt.Errorf("restore error: %v", err)
		}
	}

	log.Log(log.Always, "done")
	return nil
}

func (restore *MongoRestore) getArchiveReader() (rc io.ReadCloser, err error) {
	if restore.InputOptions.Archive == "-" {
		rc = os.Stdin
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
		return gzip.NewReader(rc)
	}
	return rc, nil
}
