// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongodump

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/mongo-tools/common/archive"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/dumprestore"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/util"

	"golang.org/x/exp/slices"
)

type NilPos struct{}

func (NilPos) Pos() int64 {
	return -1
}

// writeFlusher wraps an io.Writer and adds a Flush function.
type writeFlusher interface {
	Flush() error
	io.Writer
}

// writeFlushCloser is a writeFlusher implementation which exposes
// a Close function which is implemented by calling Flush.
type writeFlushCloser struct {
	writeFlusher
}

// errorReader implements io.Reader.
type errorReader struct{}

// Read on an errorReader already returns an error.
func (errorReader) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

// Close calls Flush.
func (bwc writeFlushCloser) Close() error {
	return bwc.Flush()
}

// realBSONFile implements the intents.file interface. It lets intents write to real BSON files
// ok disk via an embedded bufio.Writer
type realBSONFile struct {
	io.WriteCloser
	path string
	// errorWrite adds a Read() method to this object allowing it to be an
	// intent.file ( a ReadWriteOpenCloser )
	errorReader
	intent *intents.Intent
	NilPos
}

// Open is part of the intents.file interface. realBSONFiles need to have Open called before
// Read can be called
func (f *realBSONFile) Open() (err error) {
	if f.path == "" {
		// This should not occur normally. All realBSONFile's should have a path
		return fmt.Errorf("error creating BSON file without a path, namespace: %v",
			f.intent.Namespace())
	}
	err = os.MkdirAll(filepath.Dir(f.path), os.ModeDir|os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating directory for BSON file %v: %v",
			filepath.Dir(f.path), err)
	}

	f.WriteCloser, err = os.Create(f.path)
	if err != nil {
		return fmt.Errorf("error creating BSON file %v: %v", f.path, err)
	}

	return nil
}

// realMetadataFile implements intent.file, and corresponds to a Metadata file on disk
type realMetadataFile struct {
	io.WriteCloser
	path string
	errorReader
	// errorWrite adds a Read() method to this object allowing it to be an
	// intent.file ( a ReadWriteOpenCloser )
	intent *intents.Intent
	NilPos
}

// Open opens the file on disk that the intent indicates. Any directories needed are created.
func (f *realMetadataFile) Open() (err error) {
	if f.path == "" {
		return fmt.Errorf("No metadata path for %v.%v", f.intent.DB, f.intent.C)
	}
	err = os.MkdirAll(filepath.Dir(f.path), os.ModeDir|os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating directory for metadata file %v: %v",
			filepath.Dir(f.path), err)
	}

	f.WriteCloser, err = os.Create(f.path)
	if err != nil {
		return fmt.Errorf("error creating metadata file %v: %v", f.path, err)
	}
	return nil
}

// stdoutFile implements the intents.file interface. stdoutFiles are used when single collections
// are written directly (non-archive-mode) to standard out, via "--dir -"
type stdoutFile struct {
	io.Writer
	errorReader
	NilPos
}

// Open is part of the intents.file interface.
func (f *stdoutFile) Open() error {
	return nil
}

// Close is part of the intents.file interface. While we could actually close os.Stdout here,
// that's actually a bad idea. Unsetting f.File here will cause future Writes to fail, which
// is all we want.
func (f *stdoutFile) Close() error {
	f.Writer = nil
	return nil
}

// shouldSkipSystemNamespace returns true when a namespace (database +
// collection name) match certain reserved system namespaces that must
// not be dumped.
// By default dumping the entire cluster will only dump config collections
// in dumprestore.ConfigCollectionsToKeep. Every other config collection is ignoered.
// If you set --db=config then everything is included.
// If you set --db=config --collection=foo, then shouldSkipSystemNamespace() is
// never hit since CreateCollectionIntent() is run directly. In this case
// config.foo will be the olny collection dumped.
func (dump *MongoDump) shouldSkipSystemNamespace(dbName, collName string) bool {
	// ignore <db>.system.* except for admin; ignore other specific
	// collections in config and admin databases used for 3.6 features.
	switch dbName {
	case "admin":
		if collName == "system.keys" {
			return true
		}
	case "config":
		if dump.ToolOptions.DB == "config" {
			return false
		}
		return !slices.Contains(dumprestore.ConfigCollectionsToKeep, collName)
	default:
		if collName == "system.js" {
			return false
		}
		if strings.HasPrefix(collName, "system.") {
			return true
		}
	}

	// Skip over indexes since they are also listed in system.namespaces in 2.6 or earlier
	if strings.Contains(collName, "$") && !strings.Contains(collName, ".oplog.$") {
		return true
	}

	return false
}

func isReshardingCollection(collName string) bool {
	switch collName {
	case "reshardingOperations", "localReshardingOperations.donor", "localReshardingOperations.recipient":
		return true
	default:
		return false
	}
}

// shouldSkipCollection returns true when a collection name is excluded
// by the mongodump options.
func (dump *MongoDump) shouldSkipCollection(colName string) bool {
	for _, excludedCollection := range dump.OutputOptions.ExcludedCollections {
		if colName == excludedCollection {
			return true
		}
	}
	for _, excludedCollectionPrefix := range dump.OutputOptions.ExcludedCollectionPrefixes {
		if strings.HasPrefix(colName, excludedCollectionPrefix) {
			return true
		}
	}
	return false
}

// outputPath creates a path for the collection to be written to (sans file extension).
func (dump *MongoDump) outputPath(dbName, colName string) string {
	var root string
	if dump.OutputOptions.Out == "" {
		root = "dump"
	} else {
		root = dump.OutputOptions.Out
	}

	// Encode a new output path for collection names that would result in a file name greater
	// than 255 bytes long. This includes the longest possible file extension: .metadata.json.gz
	// The new format is <truncated-url-encoded-collection-name>%24<collection-name-hash-base64>
	// where %24 represents a $ symbol delimiter (e.g. aVeryVery...VeryLongName%24oPpXMQ...).
	escapedColName := util.EscapeCollectionName(colName)
	if len(escapedColName) > 238 {
		colNameTruncated := escapedColName[:208]
		colNameHashBytes := sha1.Sum([]byte(colName))
		colNameHashBase64 := base64.RawURLEncoding.EncodeToString(colNameHashBytes[:])

		// First 208 bytes of col name + 3 bytes delimiter + 27 bytes base64 hash = 238 bytes max.
		escapedColName = colNameTruncated + "%24" + colNameHashBase64
	}

	return filepath.Join(root, dbName, escapedColName)
}

// CreateOplogIntents creates an intents.Intent for the oplog and adds it to the manager
func (dump *MongoDump) CreateOplogIntents() error {
	err := dump.determineOplogCollectionName()
	if err != nil {
		return err
	}
	oplogIntent := &intents.Intent{
		DB: "",
		C:  "oplog",
	}
	if dump.OutputOptions.Archive != "" {
		oplogIntent.BSONFile = &archive.MuxIn{Mux: dump.archive.Mux, Intent: oplogIntent}
	} else {
		oplogIntent.BSONFile = &realBSONFile{path: dump.outputPath("oplog.bson", ""), intent: oplogIntent}
	}
	dump.manager.Put(oplogIntent)
	return nil
}

// CreateUsersRolesVersionIntentsForDB create intents to be written in to the specific
// database folder, for the users, roles and version admin database collections
// And then it adds the intents in to the manager
func (dump *MongoDump) CreateUsersRolesVersionIntentsForDB(db string) error {

	outDir := dump.outputPath(db, "")

	usersIntent := &intents.Intent{
		DB: db,
		C:  "$admin.system.users",
	}
	rolesIntent := &intents.Intent{
		DB: db,
		C:  "$admin.system.roles",
	}
	versionIntent := &intents.Intent{
		DB: db,
		C:  "$admin.system.version",
	}
	if dump.OutputOptions.Archive != "" {
		usersIntent.BSONFile = &archive.MuxIn{Intent: usersIntent, Mux: dump.archive.Mux}
		rolesIntent.BSONFile = &archive.MuxIn{Intent: rolesIntent, Mux: dump.archive.Mux}
		versionIntent.BSONFile = &archive.MuxIn{Intent: versionIntent, Mux: dump.archive.Mux}
	} else {
		usersIntent.BSONFile = &realBSONFile{path: filepath.Join(outDir, nameGz(dump.OutputOptions.Gzip, "$admin.system.users.bson")), intent: usersIntent}
		rolesIntent.BSONFile = &realBSONFile{path: filepath.Join(outDir, nameGz(dump.OutputOptions.Gzip, "$admin.system.roles.bson")), intent: rolesIntent}
		versionIntent.BSONFile = &realBSONFile{path: filepath.Join(outDir, nameGz(dump.OutputOptions.Gzip, "$admin.system.version.bson")), intent: versionIntent}
	}
	dump.manager.Put(usersIntent)
	dump.manager.Put(rolesIntent)
	dump.manager.Put(versionIntent)

	return nil
}

// CreateCollectionIntent builds an intent for a given collection and
// puts it into the intent manager. It should only be called when a specific
// collection is specified by --db and --collection.
func (dump *MongoDump) CreateCollectionIntent(dbName, colName string) error {
	if dump.shouldSkipCollection(colName) {
		log.Logvf(log.DebugLow, "skipping dump of %v.%v, it is excluded", dbName, colName)
		return nil
	}

	session, err := dump.SessionProvider.GetSession()
	if err != nil {
		return err
	}

	collOptions, err := db.GetCollectionInfo(session.Database(dbName).Collection(colName))
	if err != nil {
		return fmt.Errorf("error getting collection options: %v", err)
	}

	intent, err := dump.NewIntentFromOptions(dbName, collOptions)
	if err != nil {
		return err
	}

	dump.manager.Put(intent)
	return nil
}

func (dump *MongoDump) NewIntentFromOptions(dbName string, ci *db.CollectionInfo) (*intents.Intent, error) {
	intent := &intents.Intent{
		DB:      dbName,
		C:       ci.Name,
		Options: ci.Options,
		Type:    ci.Type,
	}

	// Populate the intent with the collection UUID or the empty string
	intent.UUID = ci.GetUUID()

	// Setup output location
	if dump.OutputOptions.Out == "-" { // regular standard output
		intent.BSONFile = &stdoutFile{Writer: dump.OutputWriter}
	} else {
		// Set the BSONFile path.
		if dump.OutputOptions.Archive != "" {
			// if archive mode, then the output should be written using an output
			// muxer.
			intent.BSONFile = &archive.MuxIn{Intent: intent, Mux: dump.archive.Mux}
			if dump.OutputOptions.Archive == "-" {
				intent.Location = "archive on stdout"
			} else {
				intent.Location = fmt.Sprintf("archive '%v'", dump.OutputOptions.Archive)
			}
		} else if ci.IsTimeseries() {
			path := nameGz(dump.OutputOptions.Gzip, dump.outputPath(dbName, "system.buckets."+ci.Name)+".bson")
			intent.BSONFile = &realBSONFile{path: path, intent: intent}
			intent.Location = path
		} else if ci.IsView() && !dump.OutputOptions.ViewsAsCollections {
			log.Logvf(log.DebugLow, "not dumping data for %v.%v because it is a view", dbName, ci.Name)
		} else {
			// otherwise, if it's either not a view or we're treating views as collections
			// then create a standard filesystem path for this collection.
			path := nameGz(dump.OutputOptions.Gzip, dump.outputPath(dbName, ci.Name)+".bson")
			intent.BSONFile = &realBSONFile{path: path, intent: intent}
			intent.Location = path
		}

		if dump.OutputOptions.ViewsAsCollections && ci.IsView() {
			bsonutil.RemoveKey("viewOn", &intent.Options)
			bsonutil.RemoveKey("pipeline", &intent.Options)
		}
		//Set the MetadataFile path.
		if dump.OutputOptions.Archive != "" {
			intent.MetadataFile = &archive.MetadataFile{
				Intent: intent,
				Buffer: &bytes.Buffer{},
			}
		} else {
			path := nameGz(dump.OutputOptions.Gzip, dump.outputPath(dbName, ci.Name)+".metadata.json")
			intent.MetadataFile = &realMetadataFile{path: path, intent: intent}
		}
	}

	// get a document count for scheduling purposes.
	// skips this if it is a view, as it may be incredibly slow if the
	// view is based on a slow query.

	if ci.IsView() || ci.IsTimeseries() {
		return intent, nil
	}

	session, err := dump.SessionProvider.GetSession()
	if err != nil {
		return nil, err
	}
	log.Logvf(log.DebugHigh, "Getting estimated count for %v.%v", dbName, ci.Name)
	count, err := session.Database(dbName).Collection(ci.Name).EstimatedDocumentCount(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error counting %v: %v", intent.Namespace(), err)
	}
	intent.Size = int64(count)
	return intent, nil
}

// CreateIntentsForDatabase iterates through collections in a db
// and builds dump intents for each collection.
func (dump *MongoDump) CreateIntentsForDatabase(dbName string) error {
	// we must ensure folders for empty databases are still created, for legacy purposes

	session, err := dump.SessionProvider.GetSession()
	if err != nil {
		return err
	}

	colsIter, err := db.GetCollections(session.Database(dbName), "")
	if err != nil {
		return fmt.Errorf("error getting collections for database `%v`: %v", dbName, err)
	}
	defer colsIter.Close(context.Background())

	for colsIter.Next(nil) {
		collInfo := &db.CollectionInfo{}
		err = colsIter.Decode(collInfo)
		if err != nil {
			return fmt.Errorf("error decoding collection info: %v", err)
		}

		// This MUST precede the remaining checks since it avoids
		// a mid-reshard backup.
		if dbName == "config" && dump.OutputOptions.Oplog && isReshardingCollection(collInfo.Name) {
			return fmt.Errorf("detected resharding in progress. Cannot dump with --oplog while resharding")
		}

		if dump.shouldSkipSystemNamespace(dbName, collInfo.Name) {
			log.Logvf(log.DebugHigh, "will not dump system collection '%s.%s'", dbName, collInfo.Name)
			continue
		}

		if dump.shouldSkipCollection(collInfo.Name) {
			log.Logvf(log.DebugLow, "skipping dump of %v.%v, it is excluded", dbName, collInfo.Name)
			continue
		}

		if dump.OutputOptions.ViewsAsCollections && !collInfo.IsView() {
			log.Logvf(log.DebugLow, "skipping dump of %v.%v because it is not a view", dbName, collInfo.Name)
			continue
		}
		intent, err := dump.NewIntentFromOptions(dbName, collInfo)
		if err != nil {
			return err
		}
		dump.manager.Put(intent)
	}
	return colsIter.Err()
}

func (dump *MongoDump) GetValidDbs() ([]string, error) {
	var validDbs []string
	dump.SessionProvider.GetSession()
	dbs, err := dump.SessionProvider.DatabaseNames()
	if err != nil {
		return nil, fmt.Errorf("error getting database names: %v", err)
	}
	log.Logvf(log.DebugHigh, "found databases: %v", strings.Join(dbs, ", "))

	for _, dbName := range dbs {
		if dbName == "local" {
			// local can only be explicitly dumped
			continue
		}
		if dbName == "admin" && dump.isAtlasProxy {
			// admin can't be dumped if the cluster is connected via atlas proxy
			continue
		}

		validDbs = append(validDbs, dbName)
	}

	return validDbs, nil
}

// CreateAllIntents iterates through all dbs and collections and builds
// dump intents for each collection. Returns the db names that the intents
// are created from.
func (dump *MongoDump) CreateAllIntents() error {
	dbs, err := dump.GetValidDbs()
	if err != nil {
		return err
	}
	for _, dbName := range dbs {
		if err := dump.CreateIntentsForDatabase(dbName); err != nil {
			return fmt.Errorf("error creating intents for database %s: %v", dbName, err)
		}
	}
	return nil
}

func nameGz(gz bool, name string) string {
	if gz {
		return name + ".gz"
	}
	return name
}
