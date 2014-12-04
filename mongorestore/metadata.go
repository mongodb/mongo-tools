package mongorestore

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/intents"
	"github.com/mongodb/mongo-tools/common/json"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"os"
	"strings"
)

const Users = "users"
const Roles = "roles"

type Metadata struct {
	Options bson.D          `json:"options,omitempty"`
	Indexes []IndexDocument `json:"indexes"`
}

// this struct is used to read in the options of a set of indexes
type metaDataMapIndex struct {
	Indexes []bson.M `json:"indexes"`
}

type IndexDocument struct {
	Options bson.M `bson:",inline"`
	Key     bson.D `bson:"key"`
}

// MetadataFromJSON takes a slice of JSON bytes and unmarshals them into usable
// collection options and indexes for restoring collections.
func (restore *MongoRestore) MetadataFromJSON(jsonBytes []byte) (bson.D, []IndexDocument, error) {
	meta := &Metadata{}
	err := json.Unmarshal(jsonBytes, meta)
	if err != nil {
		return nil, nil, err
	}

	// first get the ordered key information for each index,
	// then merge it with a set of options stored as a map
	metaAsMap := metaDataMapIndex{}
	err = json.Unmarshal(jsonBytes, &metaAsMap)
	if err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling metadata as map: %v", err)
	}
	for i := range meta.Indexes {
		// remove "key" and "v" from the map versions
		delete(metaAsMap.Indexes[i], "key")
		meta.Indexes[i].Options = metaAsMap.Indexes[i]
	}

	return meta.Options, meta.Indexes, nil
}

//TODO test this
func (restore *MongoRestore) IndexesFromBSON(intent *intents.Intent, bsonFile string) ([]IndexDocument, error) {
	log.Logf(log.DebugLow, "scanning %v for indexes on %v collections", bsonFile, intent.C)

	rawFile, err := os.Open(bsonFile)
	if err != nil {
		return nil, fmt.Errorf("error reading index bson file %v: %v", bsonFile, err)
	}

	bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(rawFile))
	defer bsonSource.Close()

	// iterate over stored indexes, saving all that match the collection
	indexDocument := &IndexDocument{}
	collectionIndexes := []IndexDocument{}
	for bsonSource.Next(indexDocument) {
		namespace := indexDocument.Options["ns"].(string)
		if stripDBFromNS(namespace) == intent.C {
			log.Logf(log.DebugHigh, "\tfound index %v", indexDocument.Options["name"])
			collectionIndexes = append(collectionIndexes, *indexDocument)
		}
	}
	if bsonSource.Err() != nil {
		return nil, fmt.Errorf("error scanning system.indexes for %v indexes: %v", intent.C, err)
	}

	return collectionIndexes, nil
}

func stripDBFromNS(ns string) string {
	i := strings.Index(ns, ".")
	if i > 0 && i < len(ns) {
		return ns[i+1:]
	}
	return ns
}

// CollectionExists wraps mgo's CollectionNames() method to detect if the
// given intent's collection exists
func (restore *MongoRestore) CollectionExists(intent *intents.Intent) (bool, error) {
	restore.knownCollectionsMutex.Lock()
	defer restore.knownCollectionsMutex.Unlock()

	// make sure the map exists
	if restore.knownCollections == nil {
		restore.knownCollections = map[string][]string{}
	}

	// first check if we haven't done listCollections for this database already
	if restore.knownCollections[intent.DB] == nil {
		// if the database name isn't in the cache, grab collection
		// names from the server
		session, err := restore.SessionProvider.GetSession()
		if err != nil {
			return false, fmt.Errorf("error establishing connection: %v", err)
		}
		session.SetSocketTimeout(0)
		defer session.Close()
		collections, err := session.DB(intent.DB).CollectionNames()
		if err != nil {
			return false, err
		}
		// update the cache
		restore.knownCollections[intent.DB] = collections
	}

	// now check the cache for the given collection name
	exists := util.StringSliceContains(restore.knownCollections[intent.DB], intent.C)
	return exists, nil
}

// CreateIndexes takes in an intent and an array of index documents and
// attempts to create them using the createIndexes command. If that command
// fails, we fall back to individual index creation.
func (restore *MongoRestore) CreateIndexes(intent *intents.Intent, indexes []IndexDocument) error {
	// first, sanitize the indexes
	for _, index := range indexes {
		// update the namespace of the index before inserting
		index.Options["ns"] = intent.Key()

		// check for length violations before building the command
		fullIndexName := fmt.Sprintf("%v.$%v", index.Options["ns"], index.Options["name"])
		if len(fullIndexName) > 127 {
			return fmt.Errorf(
				"cannot restore index with namespace '%v': "+
					"namespace is too long (max size is 127 bytes)", fullIndexName)
		}

		// remove the index version, forcing an update,
		// unless we specifically want to keep it
		if !restore.OutputOptions.KeepIndexVersion {
			delete(index.Options, "v")
		}
	}

	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	session.SetSafe(&mgo.Safe{})
	session.SetSocketTimeout(0)
	defer session.Close()

	// then attempt the createIndexes command
	rawCommand := bson.D{
		{"createIndexes", intent.C},
		{"indexes", indexes},
	}
	results := bson.M{}
	err = session.DB(intent.DB).Run(rawCommand, &results)
	if err == nil {
		return nil
	}
	if err.Error() != "no such cmd: createIndexes" {
		return fmt.Errorf("createIndex error: %v", err)
	}

	// if we're here, the connected server does not support the command, so we fall back
	log.Log(log.Info, "\tcreateIndexes command not supported, attemping legacy index insertion")
	for _, idx := range indexes {
		log.Logf(log.Info, "\tmanually creating index %v", idx.Options["name"])
		err = restore.LegacyInsertIndex(intent, idx)
		if err != nil {
			return fmt.Errorf("error creating index %v: %v", idx.Options["name"], err)
		}
	}
	return nil
}

func (restore *MongoRestore) LegacyInsertIndex(intent *intents.Intent, index IndexDocument) error {
	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	session.SetSocketTimeout(0)
	defer session.Close()

	// overwrite safety to make sure we catch errors
	session.SetSafe(&mgo.Safe{})
	indexCollection := session.DB(intent.DB).C("system.indexes")
	err = indexCollection.Insert(index)
	if err != nil {
		return fmt.Errorf("insert error: %v", err)
		//TODO, more error checking? Audit this...
	}

	return nil
}

func (restore *MongoRestore) CreateCollection(intent *intents.Intent, options bson.D) error {
	jsonCommand, err := bsonutil.ConvertBSONValueToJSON(
		append(bson.D{{"create", intent.C}}, options...),
	)
	if err != nil {
		return err
	}

	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	session.SetSocketTimeout(0)
	defer session.Close()

	res := bson.M{}
	err = session.DB(intent.DB).Run(jsonCommand, &res)
	if err != nil {
		return fmt.Errorf("error running create command: %v", err)
	}
	if util.IsFalsy(res["ok"]) {
		return fmt.Errorf("create command: %v", res["errmsg"])
	}
	return nil
}

func (restore *MongoRestore) RestoreUsersOrRoles(collectionType string, intent *intents.Intent) error {
	log.Logf(log.Always, "restoring %v from %v", collectionType, intent.BSONPath)

	var tempCol, tempColCommandField string
	switch collectionType {
	case Users:
		tempCol = restore.tempUsersCol
		tempColCommandField = "tempUsersCollection"
	case Roles:
		tempCol = restore.tempRolesCol
		tempColCommandField = "tempRolesCollection"
	default:
		return fmt.Errorf("cannot use %v as a collection type in RestoreUsersOrRoles", collectionType)
	}

	rawFile, err := os.Open(intent.BSONPath)
	if err != nil {
		return fmt.Errorf("error reading index bson file %v: %v", intent.BSONPath, err)
	}
	bsonSource := db.NewDecodedBSONSource(db.NewBSONSource(rawFile))
	defer bsonSource.Close()

	tempColExists, err := restore.CollectionExists(&intents.Intent{DB: "admin", C: tempCol})
	if err != nil {
		return err
	}
	if tempColExists {
		return fmt.Errorf("temporary collection admin.%v already exists", tempCol) //TODO(erf) make this more helpful
	}

	log.Logf(log.DebugLow, "restoring %v to temporary collection", collectionType)
	err = restore.RestoreCollectionToDB("admin", tempCol, bsonSource, 0)
	if err != nil {
		return fmt.Errorf("error restoring %v: %v", collectionType, err)
	}

	// make sure we always drop the temporary collection
	defer func() {
		session, err := restore.SessionProvider.GetSession()
		if err != nil {
			// logging errors here because this has no way of returning that doesn't mask other errors
			log.Logf(log.Always, "error establishing connection to drop temporary collection %v: %v", tempCol, err)
			return
		}
		session.SetSocketTimeout(0)
		defer session.Close()
		log.Logf(log.DebugHigh, "dropping temporary collection %v", tempCol)
		err = session.DB("admin").C(tempCol).DropCollection()
		if err != nil {
			log.Logf(log.Always, "error dropping temporary collection %v: %v", tempCol, err)
		}
	}()

	// If we are restoring a single database (--restoreDBUsersAndRoles), then the
	// target database will be that database, and the _mergeAuthzCollections command
	// will only restore users/roles of that database. If we are restoring the admin db or
	// doing a full restore, we tell the command to merge users/roles of all databases.
	userTargetDB := intent.DB
	if userTargetDB == "admin" {
		// _mergeAuthzCollections uses an empty db string as a sentinel for "all databases"
		userTargetDB = ""
	}

	// we have to manually convert mgo's safety to a writeconcern object
	writeConcern := bson.M{}
	if restore.safety == nil {
		writeConcern["w"] = 0
	} else {
		if restore.safety.WMode == "majority" {
			writeConcern["w"] = "majority"
		} else {
			writeConcern["w"] = restore.safety.W
		}
	}

	command := bsonutil.MarshalD{
		{"_mergeAuthzCollections", 1},
		{tempColCommandField, "admin." + tempCol},
		{"drop", restore.OutputOptions.Drop},
		{"writeConcern", writeConcern},
		{"db", userTargetDB},
	}

	session, err := restore.SessionProvider.GetSession()
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	defer session.Close()
	session.SetSocketTimeout(0)

	log.Logf(log.DebugLow, "merging %v from temp collection", collectionType)
	res := bson.M{}
	err = session.Run(command, &res)
	if err != nil {
		return fmt.Errorf("error running merge command: %v", err)
	}
	if util.IsFalsy(res["ok"]) {
		return fmt.Errorf("_mergeAuthzCollections command: %v", res["errmsg"])
	}
	return nil
}
