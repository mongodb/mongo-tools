// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package mongoimport allows importing content from a JSON, CSV, or TSV into a MongoDB instance.
package mongoimport

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/tomb.v2"
)

// Input format types accepted by mongoimport.
const (
	CSV  = "csv"
	TSV  = "tsv"
	JSON = "json"
)

// Modes accepted by mongoimport.
const (
	modeInsert = "insert"
	modeUpsert = "upsert"
	modeMerge  = "merge"
	modeDelete = "delete"
)

const (
	workerBufferSize  = 16
	progressBarLength = 24
)

// MongoImport is a container for the user-specified options and
// internal state used for running mongoimport.
type MongoImport struct {
	// processedCount keeps track of how many documents have successfully
	// been processed (inserted, upserted, merged, or deleted)
	// updated atomically, aligned at the beginning of the struct
	processedCount uint64

	// failureCount keeps track of how many documents have failed to be inserted into the database.
	// Should be updated atomically.
	failureCount uint64

	// generic mongo tool options
	ToolOptions *options.ToolOptions

	// InputOptions defines options used to read data to be ingested
	InputOptions *InputOptions

	// IngestOptions defines options used to ingest data into MongoDB
	IngestOptions *IngestOptions

	// SessionProvider is used for connecting to the database
	SessionProvider *db.SessionProvider

	// the tomb is used to synchronize ingestion goroutines and causes
	// other sibling goroutines to terminate immediately if one errors out
	tomb.Tomb

	// fields to use for upsert operations
	upsertFields []string

	// type of node the SessionProvider is connected to
	nodeType db.NodeType
}

type InputReader interface {
	// StreamDocument takes a boolean indicating if the documents should be streamed
	// in read order and a channel on which to stream the documents processed from
	// the underlying reader.  Returns a non-nil error if encountered.
	StreamDocument(ordered bool, read chan bson.D) error

	// ReadAndValidateHeader reads the header line from the InputReader and returns
	// a non-nil error if the fields from the header line are invalid; returns
	// nil otherwise. No-op for JSON input readers.
	ReadAndValidateHeader() error

	// ReadAndValidateTypedHeader is the same as ReadAndValidateHeader,
	// except it also parses types from the fields of the header. Parse errors
	// will be handled according parseGrace.
	ReadAndValidateTypedHeader(parseGrace ParseGrace) error

	// embedded io.Reader that tracks number of bytes read, to allow feeding into progress bar.
	sizeTracker
}

// New constructs a new MongoImport instance from the provided options. This will fail if the options are invalid or if
// it cannot establish a new connection to the server.
func New(opts Options) (*MongoImport, error) {
	mi := &MongoImport{
		ToolOptions:   opts.ToolOptions,
		InputOptions:  opts.InputOptions,
		IngestOptions: opts.IngestOptions,
	}
	if err := mi.validateSettings(); err != nil {
		return nil, fmt.Errorf("error validating settings: %v", err)
	}

	sessionProvider, err := db.NewSessionProvider(*opts.ToolOptions)
	if err != nil {
		return nil, fmt.Errorf("error connecting to host: %v", err)
	}

	mi.SessionProvider = sessionProvider
	return mi, nil
}

// Close disconnects the server.
func (imp *MongoImport) Close() {
	imp.SessionProvider.Close()
}

// validateSettings ensures that the tool specific options supplied for
// MongoImport are valid.
func (imp *MongoImport) validateSettings() error {
	// namespace must have a valid database; if none is specified, use 'test'
	if imp.ToolOptions.DB == "" {
		imp.ToolOptions.DB = "test"
	}
	err := util.ValidateDBName(imp.ToolOptions.DB)
	if err != nil {
		return fmt.Errorf("invalid database name: %v", err)
	}

	imp.InputOptions.Type = strings.ToLower(imp.InputOptions.Type)
	// use JSON as default input type
	if imp.InputOptions.Type == "" {
		imp.InputOptions.Type = JSON
	} else {
		if imp.InputOptions.Type != TSV &&
			imp.InputOptions.Type != JSON &&
			imp.InputOptions.Type != CSV {
			return fmt.Errorf("unknown type %v", imp.InputOptions.Type)
		}
	}

	// validate --dir flag early, before type-specific validations
	if imp.InputOptions.Dir != "" {
		if imp.InputOptions.Type != JSON {
			return fmt.Errorf("--dir can only be used with JSON input type")
		}
		if imp.InputOptions.JSONArray {
			return fmt.Errorf("cannot use --jsonArray with --dir")
		}
	}

	// ensure headers are supplied for CSV/TSV
	if imp.InputOptions.Type == CSV ||
		imp.InputOptions.Type == TSV {
		if !imp.InputOptions.HeaderLine {
			if imp.InputOptions.Fields == nil &&
				imp.InputOptions.FieldFile == nil {
				return fmt.Errorf(
					"must specify --fields, --fieldFile or --headerline to import this file type",
				)
			}
			if imp.InputOptions.FieldFile != nil &&
				*imp.InputOptions.FieldFile == "" {
				return fmt.Errorf("--fieldFile cannot be empty string")
			}
			if imp.InputOptions.Fields != nil &&
				imp.InputOptions.FieldFile != nil {
				return fmt.Errorf("incompatible options: --fields and --fieldFile")
			}
		} else {
			if imp.InputOptions.Fields != nil {
				return fmt.Errorf("incompatible options: --fields and --headerline")
			}
			if imp.InputOptions.FieldFile != nil {
				return fmt.Errorf("incompatible options: --fieldFile and --headerline")
			}
		}

		if _, err := ValidatePG(imp.InputOptions.ParseGrace); err != nil {
			return err
		}
		if imp.InputOptions.Legacy {
			return fmt.Errorf("cannot use --legacy if input type is not JSON")
		}
	} else {
		// input type is JSON
		if imp.InputOptions.HeaderLine {
			return fmt.Errorf("cannot use --headerline when input type is JSON")
		}
		if imp.InputOptions.Fields != nil {
			return fmt.Errorf("cannot use --fields when input type is JSON")
		}
		if imp.InputOptions.FieldFile != nil {
			return fmt.Errorf("cannot use --fieldFile when input type is JSON")
		}
		if imp.IngestOptions.IgnoreBlanks {
			return fmt.Errorf("cannot use --ignoreBlanks when input type is JSON")
		}
		if imp.InputOptions.ColumnsHaveTypes {
			return fmt.Errorf("cannot use --columnsHaveTypes when input type is JSON")
		}
	}

	// deprecated
	if imp.IngestOptions.Upsert {
		imp.IngestOptions.Mode = modeUpsert
	}

	// parse UpsertFields, may set default mode to modeUpsert
	if imp.IngestOptions.UpsertFields != "" {
		switch imp.IngestOptions.Mode {
		case "":
			imp.IngestOptions.Mode = modeUpsert
		case modeInsert:
			return fmt.Errorf("cannot use --upsertFields with --mode=insert")
		}
		imp.upsertFields = strings.Split(imp.IngestOptions.UpsertFields, ",")
		if err := validateFields(imp.upsertFields, imp.InputOptions.UseArrayIndexFields); err != nil {
			return fmt.Errorf("invalid --upsertFields argument: %v", err)
		}
	} else if imp.IngestOptions.Mode != modeInsert {
		imp.upsertFields = []string{"_id"}
	}

	// set default mode, must be after parsing UpsertFields
	if imp.IngestOptions.Mode == "" {
		imp.IngestOptions.Mode = modeInsert
	}

	// double-check mode choices
	if imp.IngestOptions.Mode != modeInsert &&
		imp.IngestOptions.Mode != modeUpsert &&
		imp.IngestOptions.Mode != modeDelete &&
		imp.IngestOptions.Mode != modeMerge {
		return fmt.Errorf("invalid --mode argument: %v", imp.IngestOptions.Mode)
	}

	if imp.IngestOptions.Mode != modeInsert {
		imp.IngestOptions.MaintainInsertionOrder = true
		log.Logvf(log.Info, "using upsert fields: %v", imp.upsertFields)
	}

	if imp.IngestOptions.MaintainInsertionOrder {
		imp.IngestOptions.StopOnError = true
		imp.IngestOptions.NumInsertionWorkers = 1
	} else {
		// set the number of decoding workers to use for imports
		if imp.IngestOptions.NumDecodingWorkers <= 0 {
			imp.IngestOptions.NumDecodingWorkers = imp.ToolOptions.MaxProcs
		}
		// set the number of insertion workers to use for imports
		if imp.IngestOptions.NumInsertionWorkers <= 0 {
			imp.IngestOptions.NumInsertionWorkers = 1
		}
	}
	log.Logvf(log.DebugLow, "using %v decoding workers", imp.IngestOptions.NumDecodingWorkers)
	log.Logvf(log.DebugLow, "using %v insert workers", imp.IngestOptions.NumInsertionWorkers)

	// get the number of documents per batch
	if imp.IngestOptions.BulkBufferSize <= 0 || imp.IngestOptions.BulkBufferSize > 1000 {
		imp.IngestOptions.BulkBufferSize = 1000
	}

	// ensure we have a valid string to use for the collection
	if imp.ToolOptions.Collection == "" {
		log.Logvf(log.Always, "no collection specified")
		// For directory imports, use the directory name as collection
		if imp.InputOptions.Dir != "" {
			dirBaseName := filepath.Base(imp.InputOptions.Dir)
			log.Logvf(log.Always, "using directory name '%v' as collection", dirBaseName)
			imp.ToolOptions.Collection = dirBaseName
		} else {
			fileBaseName := filepath.Base(imp.InputOptions.File)
			lastDotIndex := strings.LastIndex(fileBaseName, ".")
			if lastDotIndex != -1 {
				fileBaseName = fileBaseName[0:lastDotIndex]
			}
			log.Logvf(log.Always, "using filename '%v' as collection", fileBaseName)
			imp.ToolOptions.Collection = fileBaseName
		}
	}
	err = util.ValidateCollectionName(imp.ToolOptions.Collection)
	if err != nil {
		return fmt.Errorf("invalid collection name: %v", err)
	}
	return nil
}

// getSourceReader returns an io.Reader to read from the input source. Also
// returns a progress.Progressor which can be used to track progress if the
// reader supports it.
func (imp *MongoImport) getSourceReader() (io.ReadCloser, int64, error) {
	if imp.InputOptions.File != "" {
		file, err := os.Open(util.ToUniversalPath(imp.InputOptions.File))
		if err != nil {
			return nil, -1, err
		}
		fileStat, err := file.Stat()
		if err != nil {
			return nil, -1, err
		}
		log.Logvf(log.Info, "filesize: %v bytes", fileStat.Size())
		return file, fileStat.Size(), err
	}

	log.Logvf(log.Info, "reading from stdin")

	// Stdin has undefined max size, so return 0
	return os.Stdin, 0, nil
}

// getJSONFilesFromDir returns a list of JSON files from the specified directory
func (imp *MongoImport) getJSONFilesFromDir(dirPath string) ([]string, error) {
	var jsonFiles []string

	universalDirPath := util.ToUniversalPath(dirPath)
	entries, err := os.ReadDir(universalDirPath)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Accept .json files only
		if strings.HasSuffix(strings.ToLower(name), ".json") {
			fullPath := filepath.Join(universalDirPath, name)
			jsonFiles = append(jsonFiles, fullPath)
		}
	}

	if len(jsonFiles) == 0 {
		return nil, fmt.Errorf("no JSON files found in directory: %v", dirPath)
	}

	log.Logvf(log.Info, "found %v JSON file(s) in directory", len(jsonFiles))
	return jsonFiles, nil
}

// fileSizeProgressor implements Progressor to allow a sizeTracker to hook up with a
// progress.Bar instance, so that the progress bar can report the percentage of the file read.
type fileSizeProgressor struct {
	max int64
	sizeTracker
}

func (fsp *fileSizeProgressor) Progress() (int64, int64) {
	return fsp.Size(), fsp.max
}

// ImportDocuments is used to write input data to the database. It returns the
// number of documents successfully imported to the appropriate namespace,
// the number of failures, and any error encountered in doing this.
func (imp *MongoImport) ImportDocuments() (uint64, uint64, error) {
	// Handle directory imports with optimized batch loading
	if imp.InputOptions.Dir != "" {
		return imp.importFromDirectoryBatched()
	}

	// Existing single-file import path (unchanged)
	source, fileSize, err := imp.getSourceReader()
	if err != nil {
		return 0, 0, err
	}
	defer source.Close()

	inputReader, err := imp.getInputReader(source)
	if err != nil {
		return 0, 0, err
	}

	if imp.InputOptions.HeaderLine {
		if imp.InputOptions.ColumnsHaveTypes {
			err = inputReader.ReadAndValidateTypedHeader(ParsePG(imp.InputOptions.ParseGrace))
		} else {
			err = inputReader.ReadAndValidateHeader()
		}
		if err != nil {
			return 0, 0, err
		}
	}

	bar := &progress.Bar{
		Name:      fmt.Sprintf("%v.%v", imp.ToolOptions.DB, imp.ToolOptions.Collection),
		Watching:  &fileSizeProgressor{fileSize, inputReader},
		Writer:    log.Writer(0),
		BarLength: progressBarLength,
		IsBytes:   true,
	}
	bar.Start()
	defer bar.Stop()
	return imp.importDocuments(inputReader)
}

// importFromDirectoryBatched imports all JSON files from a directory using
// true batch loading - documents from multiple files are batched together
// in the same insertMany/bulkWrite operations for optimal performance.
func (imp *MongoImport) importFromDirectoryBatched() (uint64, uint64, error) {
	jsonFiles, err := imp.getJSONFilesFromDir(imp.InputOptions.Dir)
	if err != nil {
		return 0, 0, err
	}

	log.Logvf(log.Always, "importing %v JSON file(s) from directory: %v", len(jsonFiles), imp.InputOptions.Dir)

	// Connect once for all files
	session, err := imp.SessionProvider.GetSession()
	if err != nil {
		return 0, 0, err
	}

	log.Logvf(log.Always, "connected to: %v", util.SanitizeURI(imp.ToolOptions.ConnectionString))
	log.Logvf(log.Info, "ns: %v.%v", imp.ToolOptions.DB, imp.ToolOptions.Collection)

	// Check node type
	imp.nodeType, err = imp.SessionProvider.GetNodeType()
	if err != nil {
		return 0, 0, fmt.Errorf("error checking connected node type: %v", err)
	}
	log.Logvf(log.Info, "connected to node type: %v", imp.nodeType)

	// Drop collection if necessary
	if imp.IngestOptions.Drop {
		log.Logvf(log.Always, "dropping: %v.%v", imp.ToolOptions.DB, imp.ToolOptions.Collection)
		collection := session.Database(imp.ToolOptions.DB).Collection(imp.ToolOptions.Collection)
		if err := collection.Drop(context.TODO()); err != nil {
			return 0, 0, err
		}
	}

	// Single shared document channel for ALL files
	readDocs := make(chan bson.D, workerBufferSize)
	processingErrChan := make(chan error)

	// Start ingestion workers ONCE for all files
	// These will use insertMany/bulkWrite across ALL documents
	go func() {
		processingErrChan <- imp.ingestDocuments(readDocs)
	}()

	// Calculate total size for progress tracking
	var totalSize int64
	for _, filePath := range jsonFiles {
		if stat, err := os.Stat(filePath); err == nil {
			totalSize += stat.Size()
		}
	}

	// Create a combined size tracker for all files.
	// Note: InputReader.StreamDocument closes the output channel it is given.
	// For directory imports we therefore stream each file into its own channel and
	// forward into readDocs, which we control.
	combinedTracker := &multiFileSizeTracker{}

	// Progress bar for entire directory
	bar := &progress.Bar{
		Name:      fmt.Sprintf("%v.%v [%v files]", imp.ToolOptions.DB, imp.ToolOptions.Collection, len(jsonFiles)),
		Watching:  &fileSizeProgressor{totalSize, combinedTracker},
		Writer:    log.Writer(0),
		BarLength: progressBarLength,
		IsBytes:   true,
	}
	bar.Start()
	defer bar.Stop()

	// Stream documents from all files into the shared channel.
	// We cannot pass readDocs directly to StreamDocument because StreamDocument
	// closes the channel it is given.
	go func() {
		defer close(readDocs)

		for i, filePath := range jsonFiles {
			log.Logvf(log.Info, "streaming file %v/%v: %v", i+1, len(jsonFiles), filepath.Base(filePath))

			file, err := os.Open(util.ToUniversalPath(filePath))
			if err != nil {
				log.Logvf(log.Always, "error opening file %v: %v", filePath, err)
				// Ensure progress can still reach 100% when skipping files.
				if st, stErr := os.Stat(util.ToUniversalPath(filePath)); stErr == nil {
					combinedTracker.addCompleted(st.Size())
				}
				if imp.IngestOptions.StopOnError {
					processingErrChan <- err
					return
				}
				continue
			}

			fileStat, err := file.Stat()
			if err != nil {
				_ = file.Close()
				log.Logvf(log.Always, "error stating file %v: %v", filePath, err)
				// Best-effort progress accounting when file stat fails.
				if st, stErr := os.Stat(util.ToUniversalPath(filePath)); stErr == nil {
					combinedTracker.addCompleted(st.Size())
				}
				if imp.IngestOptions.StopOnError {
					processingErrChan <- err
					return
				}
				continue
			}
			combinedTracker.setCurrent(file, fileStat.Size())

			inputReader, err := imp.getInputReader(file)
			if err != nil {
				combinedTracker.markCurrentDone()
				_ = file.Close()
				log.Logvf(log.Always, "error creating input reader for %v: %v", filePath, err)
				if imp.IngestOptions.StopOnError {
					processingErrChan <- err
					return
				}
				continue
			}

			// Per-file channel that StreamDocument owns (it will close it).
			fileDocs := make(chan bson.D, workerBufferSize)
			streamErrChan := make(chan error, 1)
			go func() {
				// The `ordered` argument controls whether decoding preserves read order.
				// Respect the user's --maintainInsertionOrder preference for directory imports.
				streamErrChan <- inputReader.StreamDocument(imp.IngestOptions.MaintainInsertionOrder, fileDocs)
			}()

			for doc := range fileDocs {
				readDocs <- doc
			}
			streamErr := <-streamErrChan

			combinedTracker.markCurrentDone()
			_ = file.Close()

			if streamErr != nil {
				log.Logvf(log.Always, "error streaming from file %v: %v", filePath, streamErr)
				if imp.IngestOptions.StopOnError {
					processingErrChan <- streamErr
					return
				}
			}

			log.Logvf(log.Info, "completed streaming file: %v", filepath.Base(filePath))
		}
		processingErrChan <- nil
	}()

	// Wait for completion
	err = channelQuorumError(processingErrChan)
	processedCount := atomic.LoadUint64(&imp.processedCount)
	failureCount := atomic.LoadUint64(&imp.failureCount)

	log.Logvf(log.Always, "directory import complete: %v documents imported, %v failed", processedCount, failureCount)
	return processedCount, failureCount, err
}

// multiFileSizeTracker tracks bytes read across multiple files for progress reporting
type multiFileSizeTracker struct {
	mu              sync.Mutex
	currentFile     *os.File
	currentFileSize int64
	completedBytes  int64
}

func (m *multiFileSizeTracker) setCurrent(file *os.File, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentFile = file
	m.currentFileSize = size
}

func (m *multiFileSizeTracker) markCurrentDone() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentFile != nil {
		m.completedBytes += m.currentFileSize
		m.currentFile = nil
		m.currentFileSize = 0
	}
}

func (m *multiFileSizeTracker) addCompleted(bytes int64) {
	if bytes <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedBytes += bytes
}

func (m *multiFileSizeTracker) Size() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	total := m.completedBytes
	if m.currentFile == nil {
		return total
	}
	if pos, err := m.currentFile.Seek(0, io.SeekCurrent); err == nil {
		total += pos
	}
	return total
}

// importDocuments is a helper to ImportDocuments and does all the ingestion
// work by taking data from the inputReader source and writing it to the
// appropriate namespace. It returns the number of documents successfully
// imported to the appropriate namespace, the number of failures, and any error
// encountered in doing this.
func (imp *MongoImport) importDocuments(inputReader InputReader) (uint64, uint64, error) {
	session, err := imp.SessionProvider.GetSession()
	if err != nil {
		return 0, 0, err
	}

	log.Logvf(
		log.Always,
		"connected to: %v",
		util.SanitizeURI(imp.ToolOptions.ConnectionString),
	)

	log.Logvf(log.Info, "ns: %v.%v",
		imp.ToolOptions.DB,
		imp.ToolOptions.Collection)

	// check if the server is a replica set, mongos, or standalone
	imp.nodeType, err = imp.SessionProvider.GetNodeType()
	if err != nil {
		return 0, 0, fmt.Errorf("error checking connected node type: %v", err)
	}
	log.Logvf(log.Info, "connected to node type: %v", imp.nodeType)

	// drop the database if necessary
	if imp.IngestOptions.Drop {
		log.Logvf(log.Always, "dropping: %v.%v",
			imp.ToolOptions.DB,
			imp.ToolOptions.Collection)
		collection := session.Database(imp.ToolOptions.DB).
			Collection(imp.ToolOptions.Collection)
		if err := collection.Drop(context.TODO()); err != nil {
			return 0, 0, err
		}
	}

	readDocs := make(chan bson.D, workerBufferSize)
	processingErrChan := make(chan error)
	ordered := imp.IngestOptions.MaintainInsertionOrder

	// read and process from the input reader
	go func() {
		processingErrChan <- inputReader.StreamDocument(ordered, readDocs)
	}()

	// insert documents into the target database
	go func() {
		processingErrChan <- imp.ingestDocuments(readDocs)
	}()

	e1 := channelQuorumError(processingErrChan)
	processedCount := atomic.LoadUint64(&imp.processedCount)
	failureCount := atomic.LoadUint64(&imp.failureCount)
	return processedCount, failureCount, e1
}

// ingestDocuments accepts a channel from which it reads documents to be inserted
// into the target collection. It spreads the insert/upsert workload across one
// or more workers.
func (imp *MongoImport) ingestDocuments(readDocs chan bson.D) (retErr error) {
	numInsertionWorkers := imp.IngestOptions.NumInsertionWorkers
	if numInsertionWorkers <= 0 {
		numInsertionWorkers = 1
	}

	// Each ingest worker will return an error which will
	// be set in the following cases:
	//
	// 1. There is a problem connecting with the server
	// 2. The server becomes unreachable
	// 3. There is an insertion/update error - e.g. duplicate key
	//    error - and stopOnError is set to true

	wg := new(sync.WaitGroup)
	for i := 0; i < numInsertionWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// only set the first insertion error and cause sibling goroutines to terminate immediately
			err := imp.runInsertionWorker(readDocs)
			if err != nil && retErr == nil {
				retErr = err
				imp.Kill(err)
			}
		}()
	}
	wg.Wait()
	return
}

// runInsertionWorker is a helper to InsertDocuments - it reads document off
// the read channel and prepares then in batches for insertion into the database.
func (imp *MongoImport) runInsertionWorker(readDocs chan bson.D) (err error) {
	session, err := imp.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error connecting to mongod: %v", err)
	}
	collection := session.Database(imp.ToolOptions.DB).Collection(imp.ToolOptions.Collection)

	serverVersion, err := imp.SessionProvider.ServerVersionArray()
	if err != nil {
		return fmt.Errorf("failed to fetch server version: %w", err)
	}

	inserter := db.NewUnorderedBufferedBulkInserter(collection, imp.IngestOptions.BulkBufferSize, serverVersion).
		SetBypassDocumentValidation(imp.IngestOptions.BypassDocumentValidation).
		SetOrdered(imp.IngestOptions.MaintainInsertionOrder).
		SetUpsert(true)

readLoop:
	for {
		select {
		case document, alive := <-readDocs:
			if !alive {
				break readLoop
			}
			err := imp.importDocument(inserter, document)
			if db.FilterError(imp.IngestOptions.StopOnError, err) != nil {
				return err
			}
		case <-imp.Dying():
			return nil
		}
	}
	result, err := inserter.Flush()
	imp.updateCounts(result, err)
	return db.FilterError(imp.IngestOptions.StopOnError, err)
}

func (imp *MongoImport) updateCounts(result *mongo.BulkWriteResult, err error) {
	if result != nil {
		atomic.AddUint64(
			&imp.processedCount,
			safecast.MustConvert[uint64](
				result.InsertedCount+
					result.ModifiedCount+
					result.UpsertedCount+
					result.DeletedCount,
			),
		)
	}
	if bwe, ok := err.(mongo.BulkWriteException); ok {
		atomic.AddUint64(&imp.failureCount, uint64(len(bwe.WriteErrors)))
	}
}

func (imp *MongoImport) importDocument(inserter *db.BufferedBulkInserter, document bson.D) error {
	var result *mongo.BulkWriteResult
	var err error

	selector := constructUpsertDocument(imp.upsertFields, document)

	switch imp.IngestOptions.Mode {
	case modeInsert:
		result, err = inserter.Insert(document)
	case modeUpsert:
		if selector == nil {
			result, err = imp.fallbackToInsert(inserter, document)
		} else {
			result, err = inserter.Replace(selector, document)
		}
	case modeMerge:
		if selector == nil {
			result, err = imp.fallbackToInsert(inserter, document)
		} else {
			updateDoc := bson.D{{"$set", document}}
			result, err = inserter.Update(selector, updateDoc)
		}
	case modeDelete:
		if selector == nil {
			log.Logvf(
				log.Info,
				"Could not construct selector from %v, skipping document",
				imp.upsertFields,
			)
		} else {
			result, err = inserter.Delete(selector, document)
		}
	default:
		err = fmt.Errorf("Invalid mode: %v", imp.IngestOptions.Mode)
	}

	// Update success and failure counts
	imp.updateCounts(result, err)

	return err
}

func (imp *MongoImport) fallbackToInsert(
	inserter *db.BufferedBulkInserter,
	document bson.D,
) (result *mongo.BulkWriteResult, err error) {
	log.Logvf(
		log.Info,
		"Could not construct selector from %v, falling back to insert mode",
		imp.upsertFields,
	)
	result, err = inserter.Insert(document)
	return
}

func splitInlineHeader(header string) (headers []string) {
	var level uint8
	var currentField string
	for _, c := range header {
		if c == '(' {
			level++
		} else if c == ')' && level > 0 {
			level--
		}
		if c == ',' && level == 0 {
			headers = append(headers, currentField)
			currentField = ""
		} else {
			currentField = currentField + string(c)
		}
	}
	headers = append(headers, currentField) // add last field
	return
}

// getInputReader returns an implementation of InputReader based on the input type.
func (imp *MongoImport) getInputReader(in io.Reader) (InputReader, error) {
	var colSpecs []ColumnSpec
	var headers []string
	var err error
	if imp.InputOptions.Fields != nil {
		headers = splitInlineHeader(*imp.InputOptions.Fields)
	} else if imp.InputOptions.FieldFile != nil {
		headers, err = util.GetFieldsFromFile(*imp.InputOptions.FieldFile)
		if err != nil {
			return nil, err
		}
	}
	if imp.InputOptions.ColumnsHaveTypes {
		colSpecs, err = ParseTypedHeaders(headers, ParsePG(imp.InputOptions.ParseGrace))
		if err != nil {
			return nil, err
		}
	} else {
		colSpecs = ParseAutoHeaders(headers)
	}

	// header fields validation can only happen once we have an input reader
	if !imp.InputOptions.HeaderLine {
		if err = validateReaderFields(ColumnNames(colSpecs), imp.InputOptions.UseArrayIndexFields); err != nil {
			return nil, err
		}
	}

	out := os.Stdout

	ignoreBlanks := imp.IngestOptions.IgnoreBlanks && imp.InputOptions.Type != JSON
	switch imp.InputOptions.Type {
	case CSV:
		return NewCSVInputReader(
			colSpecs,
			in,
			out,
			imp.IngestOptions.NumDecodingWorkers,
			ignoreBlanks,
			imp.InputOptions.UseArrayIndexFields,
		), nil
	case TSV:
		return NewTSVInputReader(
			colSpecs,
			in,
			out,
			imp.IngestOptions.NumDecodingWorkers,
			ignoreBlanks,
			imp.InputOptions.UseArrayIndexFields,
		), nil
	}
	return NewJSONInputReader(
		imp.InputOptions.JSONArray,
		imp.InputOptions.Legacy,
		in,
		imp.IngestOptions.NumDecodingWorkers,
	), nil
}
