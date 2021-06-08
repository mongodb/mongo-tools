// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongorestore

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/idx"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/common/util"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const insertBufferFactor = 16

// Result encapsulates the outcome of a particular restore attempt.
type Result struct {
	Successes int64
	Failures  int64
	Err       error
}

// log pretty-prints the result, associated with restoring the given namespace
func (result *Result) log(ns string) {
	log.Logvf(log.Always, "finished restoring %v (%v %v, %v %v)",
		ns, result.Successes, util.Pluralize(int(result.Successes), "document", "documents"),
		result.Failures, util.Pluralize(int(result.Failures), "failure", "failures"))
}

// combineWith sums the successes and failures from both results and the overwrites the existing Err with the Err from
// the provided result.
func (result *Result) combineWith(other Result) {
	result.Successes += other.Successes
	result.Failures += other.Failures
	result.Err = other.Err
}

// withErr returns a copy of the current result with the provided error
func (result Result) withErr(err error) Result {
	result.Err = err
	return result
}

func NewResultFromBulkResult(result *mongo.BulkWriteResult, err error) Result {
	if result == nil {
		return Result{}
	}

	nSuccess := result.InsertedCount
	var nFailure int64

	// if a write concern error is encountered, the failure count may be inaccurate.
	if bwe, ok := err.(mongo.BulkWriteException); ok {
		nFailure = int64(len(bwe.WriteErrors))
	}

	return Result{nSuccess, nFailure, err}
}

func (restore *MongoRestore) RestoreIndexes() error {
	log.Logvf(log.DebugLow, "building indexes up to %v collections in parallel", restore.OutputOptions.NumParallelCollections)

	namespaceQueue := restore.indexCatalog.Queue()

	if restore.OutputOptions.NumParallelCollections > 0 {
		errChan := make(chan error)

		// start a goroutine for each job thread
		for i := 0; i < restore.OutputOptions.NumParallelCollections; i++ {
			go func(id int) {
				log.Logvf(log.DebugHigh, "starting index build routine with id=%v", id)
				for {
					namespace := namespaceQueue.Pop()
					if namespace == nil {
						log.Logvf(log.DebugHigh, "ending index build routine with id=%v, no more work to do", id)
						errChan <- nil // done
						return
					}
					err := restore.RestoreIndexesForNamespace(namespace)
					if err != nil {
						errChan <- err
						return
					}
				}
			}(i)
		}

		// wait until all goroutines are done or one of them errors out
		for i := 0; i < restore.OutputOptions.NumParallelCollections; i++ {
			err := <-errChan
			if err != nil {
				// Return first error we encounter
				return err
			}
		}
		return nil
	}

	// single-threaded
	for {
		namespace := namespaceQueue.Pop()
		if namespace == nil {
			break
		}
		err := restore.RestoreIndexesForNamespace(namespace)
		if err != nil {
			return err
		}
	}
	return nil
}

func (restore *MongoRestore) RestoreIndexesForNamespace(namespace *options.Namespace) error {
	var err error
	namespaceString := fmt.Sprintf("%s.%s", namespace.DB, namespace.Collection)
	indexes := restore.indexCatalog.GetIndexes(namespace.DB, namespace.Collection)

	for i, index := range indexes {
		// The index with the name "_id_" will always be the idIndex.
		if index.Options["name"].(string) == "_id_" {
			// The _id index was created when the collection was created,
			// so we do not build the index here.
			indexes = append(indexes[:i], indexes[i+1:]...)
			break
		}
	}

	if len(indexes) > 0 && !restore.OutputOptions.NoIndexRestore {
		log.Logvf(log.Always, "restoring indexes for collection %v from metadata", namespaceString)
		if restore.OutputOptions.ConvertLegacyIndexes {
			indexes = restore.convertLegacyIndexes(indexes, namespaceString)
		}
		if restore.OutputOptions.FixDottedHashedIndexes {
			fixDottedHashedIndexes(indexes)
		}
		for _, index := range indexes {
			log.Logvf(log.Always, "index: %#v", index)
		}
		err = restore.CreateIndexes(namespace.DB, namespace.Collection, indexes)
		if err != nil {
			return fmt.Errorf("%s: error creating indexes for %s: %v", namespaceString, namespaceString, err)
		}
	} else {
		log.Logvf(log.Always, "no indexes to restore for collection %v", namespaceString)
	}

	return nil
}

func (restore *MongoRestore) PopulateMetadataForIntents() error {
	intents := restore.manager.NormalIntents()

	for _, intent := range intents {
		var metadata *Metadata
		if intent.MetadataFile == nil {
			if _, ok := restore.dbCollectionIndexes[intent.DB]; ok {
				if indexes, ok := restore.dbCollectionIndexes[intent.DB][intent.C]; ok {
					log.Logvf(log.Always, "no metadata; falling back to system.indexes")
					for _, indexDefinition := range indexes {
						restore.indexCatalog.AddIndex(intent.DB, intent.C, indexDefinition)
					}
				}
			}
		} else {
			err := intent.MetadataFile.Open()
			if err != nil {
				return fmt.Errorf("could not open metadata file %v: %v", intent.MetadataLocation, err)
			}
			defer intent.MetadataFile.Close()

			log.Logvf(log.Always, "reading metadata for %v from %v", intent.Namespace(), intent.MetadataLocation)
			metadataJSON, err := ioutil.ReadAll(intent.MetadataFile)
			if err != nil {
				return fmt.Errorf("error reading metadata from %v: %v", intent.MetadataLocation, err)
			}
			metadata, err = restore.MetadataFromJSON(metadataJSON)
			if err != nil {
				return fmt.Errorf("error parsing metadata from %v: %v", intent.MetadataLocation, err)
			}
			if metadata != nil {
				intent.Options = metadata.Options.Map()

				for _, indexDefinition := range metadata.Indexes {
					restore.indexCatalog.AddIndex(intent.DB, intent.C, indexDefinition)
				}

				if _, ok := intent.Options["timeseries"]; ok {
					intent.Type = "timeseries"
				}

				restore.indexCatalog.SetCollation(intent.DB, intent.C, intent.HasSimpleCollation())

				if restore.OutputOptions.PreserveUUID {
					if metadata.UUID == "" {
						log.Logvf(log.Always, "--preserveUUID used but no UUID found in %v, generating new UUID for %v", intent.MetadataLocation, intent.Namespace())
					}
					intent.UUID = metadata.UUID
				}
			}
		}
	}
	return nil
}

// RestoreIntents iterates through all of the intents stored in the IntentManager, and restores them.
func (restore *MongoRestore) RestoreIntents() Result {
	log.Logvf(log.DebugLow, "restoring up to %v collections in parallel", restore.OutputOptions.NumParallelCollections)

	if restore.OutputOptions.NumParallelCollections > 0 {
		resultChan := make(chan Result)

		// start a goroutine for each job thread
		for i := 0; i < restore.OutputOptions.NumParallelCollections; i++ {
			go func(id int) {
				var workerResult Result
				log.Logvf(log.DebugHigh, "starting restore routine with id=%v", id)
				var ioBuf []byte
				for {
					intent := restore.manager.Pop()
					if intent == nil {
						log.Logvf(log.DebugHigh, "ending restore routine with id=%v, no more work to do", id)
						resultChan <- workerResult // done
						return
					}
					if fileNeedsIOBuffer, ok := intent.BSONFile.(intents.FileNeedsIOBuffer); ok {
						if ioBuf == nil {
							ioBuf = make([]byte, db.MaxBSONSize)
						}
						fileNeedsIOBuffer.TakeIOBuffer(ioBuf)
					}
					result := restore.RestoreIntent(intent)
					result.log(intent.Namespace())
					workerResult.combineWith(result)
					if result.Err != nil {
						resultChan <- workerResult.withErr(fmt.Errorf("%v: %v", intent.Namespace(), result.Err))
						return
					}
					restore.manager.Finish(intent)
					if fileNeedsIOBuffer, ok := intent.BSONFile.(intents.FileNeedsIOBuffer); ok {
						fileNeedsIOBuffer.ReleaseIOBuffer()
					}

				}
			}(i)
		}

		var totalResult Result
		// wait until all goroutines are done or one of them errors out
		for i := 0; i < restore.OutputOptions.NumParallelCollections; i++ {
			result := <-resultChan
			totalResult.combineWith(result)
			if totalResult.Err != nil {
				return totalResult
			}
		}
		return totalResult
	}

	var totalResult Result
	// single-threaded
	for {
		intent := restore.manager.Pop()
		if intent == nil {
			break
		}
		result := restore.RestoreIntent(intent)
		result.log(intent.Namespace())
		totalResult.combineWith(result)
		if result.Err != nil {
			return totalResult.withErr(fmt.Errorf("%v: %v", intent.Namespace(), result.Err))
		}
		restore.manager.Finish(intent)
	}
	return totalResult
}

// RestoreIntent attempts to restore a given intent into MongoDB.
func (restore *MongoRestore) RestoreIntent(intent *intents.Intent) Result {
	collectionExists, err := restore.CollectionExists(intent)
	if err != nil {
		return Result{Err: fmt.Errorf("error reading database: %v", err)}
	}

	if !restore.OutputOptions.Drop && collectionExists {
		log.Logvf(log.Always, "restoring to existing collection %v without dropping", intent.Namespace())
	}

	if restore.OutputOptions.Drop {
		if collectionExists {
			if strings.HasPrefix(intent.C, "system.") {
				log.Logvf(log.Always, "cannot drop system collection %v, skipping", intent.Namespace())
			} else {
				log.Logvf(log.Always, "dropping collection %v before restoring", intent.Namespace())
				err = restore.DropCollection(intent)
				if err != nil {
					return Result{Err: err} // no context needed
				}
				collectionExists = false
			}
		} else {
			log.Logvf(log.DebugLow, "collection %v doesn't exist, skipping drop command", intent.Namespace())
		}
	}

	logMessageSuffix := "using options from metadata"

	// first create the collection with options from the metadata file
	uuid := intent.UUID
	options := bsonutil.MtoD(intent.Options)
	if len(options) == 0 {
		logMessageSuffix = "with no metadata"
	}

	// The only way to specify options on the idIndex is at collection creation time.
	IDIndex := restore.indexCatalog.GetIndex(intent.DB, intent.C, "_id_")
	if IDIndex != nil {
		// Remove the index version (to use the default) unless otherwise specified.
		// If preserving UUID, we have to create a collection via
		// applyops, which requires the "v" key.
		if !restore.OutputOptions.KeepIndexVersion && !restore.OutputOptions.PreserveUUID {
			delete(IDIndex.Options, "v")
		}
		IDIndex.Options["ns"] = intent.Namespace()

		// If the collection has an idIndex, then we are about to create it, so
		// ignore the value of autoIndexId.
		for j, opt := range options {
			if opt.Key == "autoIndexId" {
				options = append(options[:j], options[j+1:]...)
			}
		}

		options = append(options, bson.E{"idIndex", *IDIndex})
	}

	if restore.OutputOptions.NoOptionsRestore {
		log.Logv(log.Info, "not restoring collection options")
		logMessageSuffix = "with no collection options"
		options = nil
	}

	if !collectionExists {
		log.Logvf(log.Info, "creating collection %v %s", intent.Namespace(), logMessageSuffix)
		log.Logvf(log.DebugHigh, "using collection options: %#v", options)
		err = restore.CreateCollection(intent, options, uuid)
		if err != nil {
			return Result{Err: fmt.Errorf("error creating collection %v: %v", intent.Namespace(), err)}
		}
		restore.addToKnownCollections(intent)
	} else {
		log.Logvf(log.Info, "collection %v already exists - skipping collection create", intent.Namespace())
	}

	var result Result
	if intent.BSONFile != nil {
		err = intent.BSONFile.Open()
		if err != nil {
			return Result{Err: err}
		}
		defer intent.BSONFile.Close()

		log.Logvf(log.Always, "restoring %v from %v", intent.DataNamespace(), intent.Location)

		bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(intent.BSONFile))
		defer bsonSource.Close()

		result = restore.RestoreCollectionToDB(intent.DB, intent.DataCollection(), bsonSource, intent.BSONFile, intent.Size)
		if result.Err != nil {
			result.Err = fmt.Errorf("error restoring from %v: %v", intent.Location, result.Err)
			return result
		}
	}

	return result
}

func (restore *MongoRestore) convertLegacyIndexes(indexes []*idx.IndexDocument, ns string) []*idx.IndexDocument {
	var indexKeys []bson.D
	var indexesConverted []*idx.IndexDocument
	for _, index := range indexes {
		bsonutil.ConvertLegacyIndexKeys(index.Key, ns)

		foundIdenticalIndex := false
		for _, keys := range indexKeys {
			if bsonutil.IsIndexKeysEqual(keys, index.Key) {
				foundIdenticalIndex = true
				break
			}
		}

		if foundIdenticalIndex {
			log.Logvf(log.Always, "index %v contains duplicate key with an existing index after ConvertLegacyIndexKeys, Skipping...", index.Options["name"])
			continue
		}

		indexKeys = append(indexKeys, index.Key)

		// It is preferable to use the ignoreUnknownIndexOptions on the createIndex command to
		// force the server to remove unknown options. But ignoreUnknownIndexOptions was only added in 4.1.9.
		// So for pre 3.4 indexes being added to servers < 4.1.9 we must strip the options here.
		if restore.serverVersion.LT(db.Version{4, 1, 9}) {
			bsonutil.ConvertLegacyIndexOptions(index.Options)
		}
		indexesConverted = append(indexesConverted, index)
	}
	return indexesConverted
}

func fixDottedHashedIndexes(indexes []*idx.IndexDocument) {
	for _, index := range indexes {
		fixDottedHashedIndex(index)
	}
}

// fixDottedHashedIndex fixes the issue introduced by a server bug where hashed index constraints are not
// correctly enforced under all circumstance by changing the hashed index on the dotted field to an
// ascending single field index.
func fixDottedHashedIndex(index *idx.IndexDocument) {
	indexFields := index.Key
	for i, field := range indexFields {
		fieldName := field.Key
		if strings.Contains(fieldName, ".") && field.Value == "hashed" {
			// Change the hashed index to single field index
			indexFields[i].Value = int32(1)
		}
	}
}

// RestoreCollectionToDB pipes the given BSON data into the database.
// Returns the number of documents restored and any errors that occurred.
func (restore *MongoRestore) RestoreCollectionToDB(dbName, colName string,
	bsonSource *db.DecodedBSONSource, file PosReader, fileSize int64) Result {

	var termErr error
	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return Result{Err: fmt.Errorf("error establishing connection: %v", err)}
	}

	collection := session.Database(dbName).Collection(colName)

	documentCount := int64(0)
	watchProgressor := progress.NewCounter(fileSize)
	if restore.ProgressManager != nil {
		name := fmt.Sprintf("%v.%v", dbName, colName)
		restore.ProgressManager.Attach(name, watchProgressor)
		defer restore.ProgressManager.Detach(name)
	}

	maxInsertWorkers := restore.OutputOptions.NumInsertionWorkers

	docChan := make(chan bson.Raw, insertBufferFactor)
	resultChan := make(chan Result, maxInsertWorkers)

	// stream documents for this collection on docChan
	go func() {
		for {
			doc := bsonSource.LoadNext()
			if doc == nil {
				break
			}

			if restore.terminate {
				log.Logvf(log.Always, "terminating read on %v.%v", dbName, colName)
				termErr = util.ErrTerminated
				close(docChan)
				return
			}

			rawBytes := make([]byte, len(doc))
			copy(rawBytes, doc)
			docChan <- bson.Raw(rawBytes)
			documentCount++
		}
		close(docChan)
	}()

	log.Logvf(log.DebugLow, "using %v insertion workers", maxInsertWorkers)

	for i := 0; i < maxInsertWorkers; i++ {
		go func() {
			var result Result

			bulk := db.NewUnorderedBufferedBulkInserter(collection, restore.OutputOptions.BulkBufferSize).
				SetOrdered(restore.OutputOptions.MaintainInsertionOrder)
			bulk.SetBypassDocumentValidation(restore.OutputOptions.BypassDocumentValidation)
			for rawDoc := range docChan {
				if restore.objCheck {
					result.Err = bson.Unmarshal(rawDoc, &bson.D{})
					if result.Err != nil {
						resultChan <- result
						return
					}
				}
				result.combineWith(NewResultFromBulkResult(bulk.InsertRaw(rawDoc)))
				result.Err = db.FilterError(restore.OutputOptions.StopOnError, result.Err)
				if result.Err != nil {
					resultChan <- result
					return
				}
				watchProgressor.Set(file.Pos())
			}
			// flush the remaining docs
			result.combineWith(NewResultFromBulkResult(bulk.Flush()))
			resultChan <- result.withErr(db.FilterError(restore.OutputOptions.StopOnError, result.Err))
			return
		}()

		// sleep to prevent all threads from inserting at the same time at start
		time.Sleep(10 * time.Millisecond)
	}

	var totalResult Result
	var finalErr error

	// wait until all insert jobs finish
	for done := 0; done < maxInsertWorkers; done++ {
		totalResult.combineWith(<-resultChan)
		if finalErr == nil && totalResult.Err != nil {
			finalErr = totalResult.Err
			restore.terminate = true
		}
	}

	if finalErr != nil {
		totalResult.Err = finalErr
	} else if err = bsonSource.Err(); err != nil {
		totalResult.Err = fmt.Errorf("reading bson input: %v", err)
	} else if termErr != nil {
		totalResult.Err = termErr
	}
	return totalResult
}
