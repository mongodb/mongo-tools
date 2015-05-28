package mongorestore

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/util"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// FileType describes the various types of restore documents.
type FileType uint

// File types constants used by mongorestore.
const (
	UnknownFileType FileType = iota
	BSONFileType
	MetadataFileType
)

// realBSONFile implements the intents.file interface. It lets intents read from real BSON files
// ok disk via an embedded os.File
// The Read, Write and Close methods of the intents.file interface is implemented here by the
// embedded os.File, the Write will return an error and not succeed
type realBSONFile struct {
	*os.File
	intent *intents.Intent
}

// Open is part of the intents.file interface. realBSONFiles need to be Opened before Read
// can be called on them.
func (f *realBSONFile) Open() (err error) {
	if f.intent.BSONPath == "" {
		// this error shouldn't happen normally
		return fmt.Errorf("error reading BSON file for %v", f.intent.Namespace())
	}
	f.File, err = os.Open(f.intent.BSONPath)
	if err != nil {
		return fmt.Errorf("error reading BSON file %v: %v", f.intent.BSONPath, err)
	}
	return nil
}

// realMetadataFile implements the intents.file interface. It lets intents read from real
// metadata.json files ok disk via an embedded os.File
// The Read, Write and Close methods of the intents.file interface is implemented here by the
// embedded os.File, the Write will return an error and not succeed
type realMetadataFile struct {
	*os.File
	intent *intents.Intent
}

// Open is part of the intents.file interface. realMetadataFiles need to be Opened before Read
// can be called on them.
func (f *realMetadataFile) Open() (err error) {
	if f.intent.MetadataPath == "" {
		return fmt.Errorf("error reading Metadata file for %v", f.intent.Namespace())
	}
	f.File, err = os.Open(f.intent.MetadataPath)
	if err != nil {
		return fmt.Errorf("error reading Metadata file %v: %v", f.intent.MetadataPath, err)
	}
	return nil
}

// stdinFile implements the intents.file interface. They allow intents to read single collections
// from standard input
type stdinFile struct {
	io.Reader
	intent *intents.Intent
}

// Open is part of the intents.file interface. stdinFile needs to have Open called on it before
// Read can be called on it.
func (f *stdinFile) Open() error {
	f.Reader = os.Stdin
	return nil
}

// Close is part of the intents.file interface. After Close is called, Read will fail.
func (f *stdinFile) Close() error {
	f.Reader = nil
	return nil
}

// Write is part of the intents.file interface. Nobody should be writing to stdinFiles,
// could probably justifiably panic here.
func (f *stdinFile) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("can't write to standard output")
}

// GetInfoFromFilename pulls the base collection name and FileType from a given file.
func GetInfoFromFilename(filename string) (string, FileType) {
	baseFileName := filepath.Base(filename)
	switch {
	case strings.HasSuffix(baseFileName, ".metadata.json"):
		// this logic can't be simple because technically
		// "x.metadata.json" is a valid collection name
		baseName := strings.TrimSuffix(baseFileName, ".metadata.json")
		return baseName, MetadataFileType
	case strings.HasSuffix(baseFileName, ".bin"):
		// .bin supported for legacy reasons
		baseName := strings.TrimSuffix(baseFileName, ".bin")
		return baseName, BSONFileType
	case strings.HasSuffix(baseFileName, ".bson"):
		baseName := strings.TrimSuffix(baseFileName, ".bson")
		return baseName, BSONFileType
	default:
		return "", UnknownFileType
	}
}

// CreateAllIntents drills down into a dump folder, creating intents for all of
// the databases and collections it finds.
func (restore *MongoRestore) CreateAllIntents(dir archive.DirLike, filterDB string, filterCollection string) error {
	log.Logf(log.DebugHigh, "using %v as dump root directory", dir.Path())
	foundOplog := false
	entries, err := dir.ReadDir()
	if err != nil {
		return fmt.Errorf("error reading root dump folder: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if err = util.ValidateDBName(entry.Name()); err != nil {
				return fmt.Errorf("invalid database name '%v': %v", entry.Name(), err)
			}
			if filterDB == "" || entry.Name() == filterDB {
				err = restore.CreateIntentsForDB(entry.Name(), filterCollection, entry, false)
			} else {
				err = restore.CreateIntentsForDB(entry.Name(), "", entry, true)
			}
			if err != nil {
				return err
			}
		} else {
			if entry.Name() == "oplog.bson" {
				if restore.InputOptions.OplogReplay {
					log.Log(log.DebugLow, "found oplog.bson file to replay")
				}
				foundOplog = true
				oplogIntent := &intents.Intent{
					C:        "oplog",
					BSONPath: entry.Path(),
					Size:     entry.Size(),
				}
				if filterDB != "" || filterCollection != "" {
					if restore.InputOptions.Archive == "" {
						continue
					} else {
						mutedOut := &archive.MutedCollection{
							Intent: oplogIntent,
							Demux:  restore.archive.Demux,
						}
						restore.archive.Demux.Open(
							oplogIntent.Namespace(),
							mutedOut,
						)
						continue
					}
				}
				if restore.InputOptions.Archive != "" {
					// no need to check that we want to cache here
					oplogIntent.BSONFile =
						&archive.RegularCollectionReceiver{
							Intent: oplogIntent,
							Demux:  restore.archive.Demux,
						}
				} else {
					oplogIntent.BSONFile = &realBSONFile{intent: oplogIntent}
				}
				restore.manager.Put(oplogIntent)
			} else {
				log.Logf(log.Always,
					`don't know what to do with file "%v", skipping...`,
					entry.Path(),
				)
			}
		}
	}
	if restore.InputOptions.OplogReplay && !foundOplog {
		return fmt.Errorf("no %v/oplog.bson file to replay; make sure you run mongodump with --oplog", dir.Path())
	}
	return nil
}

// CreateIntentsForDB drills down into the dir folder, creating intents
// for all of the collection dump files it finds for the db database.
func (restore *MongoRestore) CreateIntentsForDB(db string, filterCollection string, dir archive.DirLike, mute bool) (err error) {
	var entries []archive.DirLike
	log.Logf(log.DebugHigh, "reading collections for database %v in %v", db, dir.Name())
	entries, err = dir.ReadDir()
	if err != nil {
		return fmt.Errorf("error reading db folder %v: %v", db, err)
	}
	usesMetadataFiles := hasMetadataFiles(entries)
	for _, entry := range entries {
		if entry.IsDir() {
			log.Logf(log.Always, `don't know what to do with subdirectory "%v", skipping...`,
				filepath.Join(dir.Name(), entry.Name()))
		} else {
			collection, fileType := GetInfoFromFilename(entry.Name())
			switch fileType {
			case BSONFileType:
				var skip = mute
				// Dumps of a single database (i.e. with the -d flag) may contain special
				// db-specific collections that start with a "$" (for example, $admin.system.users
				// holds the users for a database that was dumped with --dumpDbUsersAndRoles enabled).
				// If these special files manage to be included in a dump directory during a full
				// (multi-db) restore, we should ignore them.
				if restore.ToolOptions.DB == "" && strings.HasPrefix(collection, "$") {
					log.Logf(log.DebugLow,
						"not restoring special collection %v.%v", db, collection)
					skip = true
				}
				// TOOLS-717: disallow restoring to the system.profile collection.
				// Server versions >= 3.0.3 disallow user inserts to system.profile so
				// it would likely fail anyway.
				if collection == "system.profile" {
					log.Logf(log.DebugLow, "skipping restore of system.profile collection", db)
					skip = true
				}
				// skip restoring the indexes collection if we are using metadata
				// files to store index information, to eliminate redundancy
				if collection == "system.indexes" && usesMetadataFiles {
					log.Logf(log.DebugLow,
						"not restoring system.indexes collection because database %v "+
							"has .metadata.json files", db)
					skip = true
				}
				if filterCollection != "" && filterCollection != collection {
					skip = true
				}
				intent := &intents.Intent{
					DB:       db,
					C:        collection,
					Size:     entry.Size(),
					BSONPath: entry.Path(),
				}
				if restore.InputOptions.Archive != "" {
					if skip {
						// adding the DemuxOut to the demux, but not adding the intent to the manager
						mutedOut := &archive.MutedCollection{Intent: intent, Demux: restore.archive.Demux}
						restore.archive.Demux.Open(intent.Namespace(), mutedOut)
						continue
					} else {
						if intent.IsSpecialCollection() {
							intent.BSONFile = &archive.SpecialCollectionCache{Intent: intent, Demux: restore.archive.Demux}
							restore.archive.Demux.Open(intent.Namespace(), intent.BSONFile)
						} else {
							intent.BSONFile = &archive.RegularCollectionReceiver{Intent: intent, Demux: restore.archive.Demux}
						}
					}
				} else {
					if skip {
						continue
					}
					if restore.useStdin {
						intent = &intents.Intent{
							DB:       db,
							C:        collection,
							BSONPath: "-",
						}
						intent.BSONFile = &stdinFile{intent: intent}
					} else {
						intent.BSONFile = &realBSONFile{intent: intent}
					}
				}
				log.Logf(log.Info, "found collection %v bson to restore", intent.Namespace())
				restore.manager.Put(intent)
			case MetadataFileType:
				usesMetadataFiles = true
				intent := &intents.Intent{
					DB:           db,
					C:            collection,
					MetadataPath: entry.Path(),
				}
				if restore.InputOptions.Archive != "" {
					intent.MetadataFile = &archive.MetadataPreludeFile{Intent: intent, Prelude: restore.archive.Prelude}
				} else {
					intent.MetadataFile = &realMetadataFile{intent: intent}
				}
				log.Logf(log.Info, "found collection %v metadata to restore", intent.Namespace())
				restore.manager.Put(intent)
			default:
				log.Logf(log.Always, `don't know what to do with file "%v", skipping...`,
					entry.Path())
			}
		}
	}
	return nil
}

// CreateIntentForCollection builds an intent for the given database and collection name
// along with a path to a .bson collection file. It searches the file's parent directory
// for a matching metadata file.
//
// This method is not called by CreateIntentsForDB,
// it is only used in the case where --db and --collection flags are set.
func (restore *MongoRestore) CreateIntentForCollection(db string, collection string, dir archive.DirLike) error {

	log.Logf(log.DebugLow, "reading collection %v for database %v from %v",
		collection, db, dir.Path())

	// avoid actual file handling if we are using stdin
	if restore.useStdin {
		intent := &intents.Intent{
			DB:       db,
			C:        collection,
			BSONPath: "-",
		}
		intent.BSONFile = &stdinFile{intent: intent}
		restore.manager.Put(intent)
		return nil
	}

	// first make sure the bson file exists and is valid
	_, err := dir.Stat()
	if err != nil {
		return err
	}
	if dir.IsDir() {
		return fmt.Errorf("file %v is a directory, not a bson file", dir.Path())
	}

	baseName, fileType := GetInfoFromFilename(dir.Name())
	if fileType != BSONFileType {
		return fmt.Errorf("file %v does not have .bson extension", dir.Path())
	}

	// then create its intent
	intent := &intents.Intent{
		DB:       db,
		C:        collection,
		BSONPath: dir.Path(),
		Size:     dir.Size(),
	}
	intent.BSONFile = &realBSONFile{intent: intent}

	// finally, check if it has a .metadata.json file in its folder
	log.Logf(log.DebugLow, "scanning directory %v for metadata file", dir.Name())
	entries, err := dir.Parent().ReadDir()
	if err != nil {
		// try and carry on if we can
		log.Logf(log.Info, "error attempting to locate metadata for file: %v", err)
		log.Log(log.Info, "restoring collection without metadata")
		restore.manager.Put(intent)
		return nil
	}
	metadataName := baseName + ".metadata.json"
	for _, entry := range entries {
		if entry.Name() == metadataName {
			metadataPath := entry.Path()
			log.Logf(log.Info, "found metadata for collection at %v", metadataPath)
			intent.MetadataPath = metadataPath
			intent.MetadataFile = &realMetadataFile{intent: intent}
			break
		}
	}

	if intent.MetadataPath == "" {
		log.Log(log.Info, "restoring collection without metadata")
	}

	restore.manager.Put(intent)

	return nil
}

// helper for searching a list of FileInfo for metadata files
func hasMetadataFiles(files []archive.DirLike) bool {
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".metadata.json") {
			return true
		}
	}
	return false
}

// handleBSONInsteadOfDirectory updates -d and -c settings based on
// the path to the BSON file passed to mongorestore. This is only
// applicable if the target path points to a .bson file.
//
// As an example, when the user passes 'dump/mydb/col.bson', this method
// will infer that 'mydb' is the database and 'col' is the collection name.
func (restore *MongoRestore) handleBSONInsteadOfDirectory(path string) error {
	// we know we have been given a non-directory, so we should handle it
	// like a bson file and infer as much as we can
	if restore.ToolOptions.Collection == "" {
		// if the user did not set -c, use the file name for the collection
		newCollectionName, fileType := GetInfoFromFilename(path)
		if fileType != BSONFileType {
			return fmt.Errorf("file %v does not have .bson extension", path)
		}
		restore.ToolOptions.Collection = newCollectionName
		log.Logf(log.DebugLow, "inferred collection '%v' from file", restore.ToolOptions.Collection)
	}
	if restore.ToolOptions.DB == "" {
		// if the user did not set -d, use the directory containing the target
		// file as the db name (as it would be in a dump directory). If
		// we cannot determine the directory name, use "test"
		dirForFile := filepath.Base(filepath.Dir(path))
		if dirForFile == "." || dirForFile == ".." {
			dirForFile = "test"
		}
		restore.ToolOptions.DB = dirForFile
		log.Logf(log.DebugLow, "inferred db '%v' from the file's directory", restore.ToolOptions.DB)
	}
	return nil
}

type actualPath struct {
	os.FileInfo
	path   string
	parent *actualPath
}

func newActualPath(dir string) (*actualPath, error) {
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Dir(filepath.Clean(dir))
	parent := &actualPath{}
	parentStat, err := os.Stat(path)
	if err == nil {
		parent.FileInfo = parentStat
		parent.path = filepath.Dir(path)
	}
	ap := &actualPath{
		FileInfo: stat,
		path:     path,
		parent:   parent,
	}
	return ap, nil
}

func (ap actualPath) Path() string {
	return filepath.Join(ap.path, ap.Name())
}

func (ap actualPath) Parent() archive.DirLike {
	// returns nil if there is no parent
	return ap.parent
}

func (ap actualPath) ReadDir() ([]archive.DirLike, error) {
	entries, err := ioutil.ReadDir(ap.Path())
	if err != nil {
		return nil, err
	}
	var returnFileInfo = make([]archive.DirLike, 0, len(entries))
	for _, entry := range entries {
		returnFileInfo = append(returnFileInfo,
			actualPath{
				FileInfo: entry,
				path:     ap.Path(),
				parent:   &ap,
			})
	}
	return returnFileInfo, nil
}

func (ap actualPath) Stat() (archive.DirLike, error) {
	stat, err := os.Stat(ap.Path())
	if err != nil {
		return nil, err
	}
	return &actualPath{FileInfo: stat, path: ap.Path()}, nil
}

func (ap actualPath) IsDir() bool {
	stat, err := os.Stat(ap.Path())
	if err != nil {
		return false
	}
	return stat.IsDir()
}
