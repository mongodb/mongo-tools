package mongodump
import (
	"fmt"
	"io/ioutil"
)

var Usage = `<options>

Export the content of a running server into .bson files.

Specify a database with -d and a collection with -c to only dump that database or collection.

See http://docs.mongodb.org/manual/reference/program/mongodump/ for more information.`

// InputOptions defines the set of options to use in retrieving data from the server.
type InputOptions struct {
	Query     	string `long:"query" short:"q" description:"query filter, as a JSON string, e.g., '{x:{$gt:1}}'"`
	QueryFile	string `long:"queryFile" description:"query filter, as a JSON file"`
	TableScan 	bool   `long:"forceTableScan" description:"force a table scan"`
}

// Name returns a human-readable group name for input options.
func (_ *InputOptions) Name() string {
	return "query"
}

func (inputOptions *InputOptions) HasQuery() (bool){
	return inputOptions.Query != "" || inputOptions.QueryFile != ""
}

func (inputOptions *InputOptions) GetQuery() ([]byte, error) {
	if inputOptions.Query != "" {
		return []byte(inputOptions.Query), nil
	} else if inputOptions.QueryFile != "" {
		content, err := ioutil.ReadFile(inputOptions.QueryFile)
		if err != nil{
			fmt.Errorf("error reading queryFile: %v", err)
		}
		return content, err
	} else {
		return nil, fmt.Errorf("GetQuery can return valid values only for query or queryFile input")
	}
}

// OutputOptions defines the set of options for writing dump data.
type OutputOptions struct {
	Out                        string   `long:"out" short:"o" description:"output directory, or '-' for stdout (defaults to 'dump')" default-mask:"-"`
	Gzip                       bool     `long:"gzip" description:"compress archive our collection output with Gzip"`
	Repair                     bool     `long:"repair" description:"try to recover documents from damaged data files (not supported by all storage engines)"`
	Oplog                      bool     `long:"oplog" description:"use oplog for taking a point-in-time snapshot"`
	Archive                    string   `long:"archive" optional:"true" optional-value:"-" description:"dump in to the specified dump-archive instead of a directory"`
	DumpDBUsersAndRoles        bool     `long:"dumpDbUsersAndRoles" description:"dump user and role definitions for the specified database"`
	ExcludedCollections        []string `long:"excludeCollection" description:"collection to exclude from the dump (may be specified multiple times to exclude additional collections)"`
	ExcludedCollectionPrefixes []string `long:"excludeCollectionsWithPrefix" description:"exclude all collections from the dump that have the given prefix (may be specified multiple times to exclude additional prefixes)"`
}

// Name returns a human-readable group name for output options.
func (_ *OutputOptions) Name() string {
	return "output"
}
