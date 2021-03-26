package idx

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// CollectionIndexCatalog stores the current view of all indexes of a single collection.
type CollectionIndexCatalog struct {
	// Maps index name to the raw index spec.
	indexes         map[string]*IndexDocument
	simpleCollation bool
}

// IndexCatalog stores the current view of all indexes in all databases.
type IndexCatalog struct {
	sync.Mutex
	// Maps database name to collection name to CollectionIndexCatalog.
	indexes map[string]map[string]*CollectionIndexCatalog
}

// NewIndexCatalog inits an IndexCatalog
func NewIndexCatalog() *IndexCatalog {
	return &IndexCatalog{indexes: make(map[string]map[string]*CollectionIndexCatalog)}
}

// Namespaces returns all the namespaces in the IndexCatalog
func (i *IndexCatalog) Namespaces() (namespaces []options.Namespace) {
	for database, dbIndexMap := range i.indexes {
		for collection := range dbIndexMap {
			namespaces = append(namespaces, options.Namespace{database, collection})
		}
	}
	return namespaces
}

func (i *IndexCatalog) getCollectionIndexes(database, collection string) map[string]*IndexDocument {
	dbIndexes, found := i.indexes[database]
	if !found {
		dbIndexes = make(map[string]*CollectionIndexCatalog)
		i.indexes[database] = dbIndexes
	}
	collIndexCatalog, found := dbIndexes[collection]
	if !found {
		collIndexCatalog = &CollectionIndexCatalog{
			indexes: make(map[string]*IndexDocument),
		}
		dbIndexes[collection] = collIndexCatalog
	}
	return collIndexCatalog.indexes
}

func (i *IndexCatalog) getCollectionIndexCatalog(database, collection string) *CollectionIndexCatalog {
	dbIndexes, found := i.indexes[database]
	if !found {
		dbIndexes = make(map[string]*CollectionIndexCatalog)
		i.indexes[database] = dbIndexes
	}
	collIndexCatalog, found := dbIndexes[collection]
	if !found {
		collIndexCatalog = &CollectionIndexCatalog{
			indexes: make(map[string]*IndexDocument),
		}
		dbIndexes[collection] = collIndexCatalog
	}
	return collIndexCatalog
}

func (i *IndexCatalog) addIndex(database, collection, indexName string, index *IndexDocument) {
	i.Lock()
	collIndexes := i.getCollectionIndexes(database, collection)
	collIndexes[indexName] = index
	i.Unlock()
}

// AddIndex stores the given index into the index catalog. An example index:
// {
// 	"v": 2,
// 	"key": {
// 		"lastModifiedDate": 1
// 	},
// 	"name": "lastModifiedDate_1",
// 	"ns": "test.eventlog"
// }
func (i *IndexCatalog) AddIndex(database, collection string, index *IndexDocument) {
	indexName, ok := index.Options["name"].(string)
	if !ok {
		return
	}
	i.addIndex(database, collection, indexName, index)
}

// SetCollation sets if a collection has a simple collation
func (i *IndexCatalog) SetCollation(database, collection string, simpleCollation bool) {
	i.Lock()
	defer i.Unlock()
	collIndexCatalog := i.getCollectionIndexCatalog(database, collection)
	collIndexCatalog.simpleCollation = simpleCollation
}

// AddIndexes stores the given indexes into the index catalog.
func (i *IndexCatalog) AddIndexes(database, collection string, indexes []*IndexDocument) {
	for _, index := range indexes {
		i.AddIndex(database, collection, index)
	}
}

// GetIndex returns an IndexDocument for a given index name
func (i *IndexCatalog) GetIndex(database, collection, indexName string) *IndexDocument {
	dbIndexes, found := i.indexes[database]
	if !found {
		return nil
	}
	collIndexCatalog, found := dbIndexes[collection]
	if !found {
		return nil
	}
	indexSpec, found := collIndexCatalog.indexes[indexName]
	if !found {
		return nil
	}
	return indexSpec
}

// String formats the IndexCatalog for debugging purposes
func (i *IndexCatalog) String() string {
	var b strings.Builder
	b.WriteString("IndexCatalog:\n")
	for dbName, coll := range i.indexes {
		for collName, collIndexCatalog := range coll {
			b.WriteString(fmt.Sprintf("\t%s.%s: \n", dbName, collName))
			for indexName, indexSpec := range collIndexCatalog.indexes {
				b.WriteString(fmt.Sprintf("\t\t%s: %+#v\n", indexName, indexSpec))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func hasCollationOnIndex(index *IndexDocument) bool {
	if _, ok := index.Options["collation"]; ok {
		return true
	}
	return false
}

// GetIndexes returns all the indexes for the given collection.
// When the collection has a non-simple collation an explicit simple collation
// must be added to the indexes with no "collation field. Otherwise, the index
// will wrongfully inherit the collections's collation.
// This is necessary because indexes with the simple collation do not have a
// "collation" field in the getIndexes output.
func (i *IndexCatalog) GetIndexes(database, collection string) []*IndexDocument {
	dbIndexes, found := i.indexes[database]
	if !found {
		return nil
	}
	collIndexCatalog, found := dbIndexes[collection]
	if !found {
		return nil
	}
	var syncedIndexes []*IndexDocument
	for _, index := range collIndexCatalog.indexes {
		if !collIndexCatalog.simpleCollation && !hasCollationOnIndex(index) {
			index.Options["collation"] = bson.D{{"locale", "simple"}}
		}
		syncedIndexes = append(syncedIndexes, index)
	}
	return syncedIndexes
}

// DropDatabase removes a database from the index catalog.
func (i *IndexCatalog) DropDatabase(database string) {
	delete(i.indexes, database)
}

// DropCollection removes a collection from the index catalog.
func (i *IndexCatalog) DropCollection(database, collection string) {
	delete(i.indexes[database], collection)
}

// DeleteIndexes removes indexes from the index catalog. dropCmd may be,
// {"deleteIndexes": "eventlog", "index": "*"}
// or,
// {"deleteIndexes": "eventlog", "index": "name_1"}
func (i *IndexCatalog) DeleteIndexes(database, collection string, dropCmd bson.D) error {
	collIndexes := i.getCollectionIndexes(database, collection)
	if len(collIndexes) == 0 {
		// We have no indexes to drop.
		return nil
	}
	indexValue, keyError := bsonutil.FindValueByKey("index", &dropCmd)
	if keyError != nil {
		return nil
	}
	switch indexToDrop := indexValue.(type) {
	case string:
		if indexToDrop == "*" {
			// Drop all non-id indexes for the collection.
			idIndex, found := collIndexes["_id_"]
			i.DropCollection(database, collection)
			if found {
				i.addIndex(database, collection, "_id_", idIndex)
			}
			return nil
		} else {
			// Drop an index by name.
			delete(collIndexes, indexToDrop)
			return nil
		}
	case bson.D:
		// Drop an index by key pattern.
		for key, value := range collIndexes {
			isEq, err := bsonutil.IsEqual(indexToDrop, value.Key)
			if err != nil {
				return fmt.Errorf("could not drop index on %s.%s, could not handle %v: "+
					"was unable to find matching index in indexCatalog. Error with equality test: %v",
					database, collection, dropCmd[0].Key, err)
			}

			if isEq {
				delete(collIndexes, key)
			}
		}
		log.Logvf(log.DebugHigh, "Must drop index on %s.%s by key pattern: %v", database, collection, indexToDrop)
		return nil
	default:
		return fmt.Errorf("could not drop index on %s.%s, could not handle %v: "+
			"expected string or object for 'index', found: %T, %v",
			database, collection, dropCmd[0].Key, indexToDrop, indexToDrop)
	}
}

func updateExpireAfterSeconds(index *IndexDocument, expire int64) error {
	if _, ok := index.Options["expireAfterSeconds"]; !ok {
		return errors.Errorf("missing \"expireAfterSeconds\" in matching index: %v", index)
	}
	index.Options["expireAfterSeconds"] = expire
	return nil
}

func updateHidden(index *IndexDocument, hidden bool) {
	index.Options["hidden"] = hidden
}

// GetIndexByIndexMod returns an index that matches the name or key pattern specified in
// a collMod command.
func (i *IndexCatalog) GetIndexByIndexMod(database, collection string, indexMod bson.D) (*IndexDocument, error) {
	// Look for "name" or "keyPattern".
	name, nameErr := bsonutil.FindStringValueByKey("name", &indexMod)
	keyPattern, keyPatternErr := bsonutil.FindSubdocumentByKey("keyPattern", &indexMod)
	switch {
	case nameErr == nil && keyPatternErr == nil:
		return nil, errors.Errorf("cannot specify both index name and keyPattern: %v", indexMod)
	case nameErr != nil && keyPatternErr != nil:
		return nil, errors.Errorf("must specify either index name (as a string) or keyPattern (as a document): %v", indexMod)
	case nameErr == nil:
		matchingIndex := i.GetIndex(database, collection, name)
		if matchingIndex == nil {
			return nil, errors.Errorf("cannot find index in indexCatalog for collMod: %v", indexMod)
		}
		return matchingIndex, nil
	case keyPatternErr == nil:
		collIndexes := i.getCollectionIndexes(database, collection)
		for _, indexSpec := range collIndexes {
			isEq, err := bsonutil.IsEqual(keyPattern, indexSpec.Key)
			if err != nil {
				return nil, fmt.Errorf("was unable to find matching index in indexCatalog. Error with equality test: %v", err)
			}
			if isEq {
				return indexSpec, nil
			}
		}
		return nil, errors.Errorf("cannot find index in indexCatalog for collMod: %v", indexMod)
	default:
		return nil, errors.Errorf("cannot find index in indexCatalog for collMod: %v", indexMod)
	}
}

func (i *IndexCatalog) collMod(database, collection string, indexModValue interface{}) error {
	indexMod, ok := indexModValue.(bson.D)
	if !ok {
		return errors.Errorf("unknown collMod \"index\" modifier: %v", indexModValue)
	}

	matchingIndex, err := i.GetIndexByIndexMod(database, collection, indexMod)
	if err != nil {
		return err
	}
	if matchingIndex == nil {
		// Did not find an index to modify.
		return errors.Errorf("cannot find index in indexCatalog for collMod: %v", indexMod)
	}

	expireValue, expireKeyError := bsonutil.FindValueByKey("expireAfterSeconds", &indexMod)
	if expireKeyError == nil {
		newExpire, ok := expireValue.(int64)
		if !ok {
			return errors.Errorf("expireAfterSeconds must be a number (found %v of type %T): %v", expireValue, expireValue, indexMod)
		}
		err = updateExpireAfterSeconds(matchingIndex, newExpire)
		if err != nil {
			return err
		}
	}

	expireValue, hiddenKeyError := bsonutil.FindValueByKey("hidden", &indexMod)
	if hiddenKeyError == nil {
		newHidden, ok := expireValue.(bool)
		if !ok {
			return errors.Errorf("hidden must be a boolean (found %v of type %T): %v", expireValue, expireValue, indexMod)
		}
		updateHidden(matchingIndex, newHidden)
	}

	if expireKeyError != nil && hiddenKeyError != nil {
		return errors.Errorf("must specify expireAfterSeconds or hidden: %v", indexMod)
	}

	// Update the index.
	i.AddIndex(database, collection, matchingIndex)
	return nil
}

// CollMod, updates the corresponding TTL index if the given collModCmd
// updates the "expireAfterSeconds" or "hiddne" fields. For example,
// {
//  "collMod": "sessions",
//  "index": {"keyPattern": {"lastAccess": 1}, "expireAfterSeconds": 3600}}
// }
// or,
// {
//  "collMod": "sessions",
//  "index": {"name": "lastAccess_1", "expireAfterSeconds": 3600}}
// }
func (i *IndexCatalog) CollMod(database, collection string, indexModValue interface{}) error {
	err := i.collMod(database, collection, indexModValue)
	if err != nil {
		return fmt.Errorf("could not handle collMod on %s.%s: %v", database, collection, err)
	}
	return nil
}

// NamespaceQueue is a goroutine-safe queue of namespaces
type NamespaceQueue struct {
	m          sync.Mutex
	namespaces []options.Namespace
}

// Queue returns a namespace queue of the current namespaces in the index catalog.
func (i *IndexCatalog) Queue() *NamespaceQueue {
	var namespaceQueue NamespaceQueue
	namespaceQueue.namespaces = i.Namespaces()
	return &namespaceQueue
}

// Pop removes the next element from the queue and returns it. It is goroutine-safe.
func (q *NamespaceQueue) Pop() *options.Namespace {
	q.m.Lock()
	defer q.m.Unlock()
	if len(q.namespaces) == 0 {
		return nil
	}
	namespace := q.namespaces[0]
	q.namespaces = q.namespaces[1:]
	return &namespace
}
