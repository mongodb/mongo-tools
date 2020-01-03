// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

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
	Query          string `long:"query" short:"q" description:"query filter, as a v2 Extended JSON string, e.g., '{\"x\":{\"$gt\":1}}'"`
	QueryFile      string `long:"queryFile" description:"path to a file containing a query filter (v2 Extended JSON)"`
	ReadPreference string `long:"readPreference" value-name:"<string>|<json>" description:"specify either a preference mode (e.g. 'nearest') or a preference json object (e.g. '{mode: \"nearest\", tagSets: [{a: \"b\"}], maxStalenessSeconds: 123}')"`
	TableScan      bool   `long:"forceTableScan" description:"force a table scan"`
}

// Name returns a human-readable group name for input options.
func (*InputOptions) Name() string {
	return "query"
}

func (inputOptions *InputOptions) HasQuery() bool {
	return inputOptions.Query != "" || inputOptions.QueryFile != ""
}

func (inputOptions *InputOptions) GetQuery() ([]byte, error) {
	if inputOptions.Query != "" {
		return []byte(inputOptions.Query), nil
	} else if inputOptions.QueryFile != "" {
		content, err := ioutil.ReadFile(inputOptions.QueryFile)
		if err != nil {
			err = fmt.Errorf("error reading queryFile: %s", err)
		}
		return content, err
	}
	panic("GetQuery can return valid values only for query or queryFile input")
}

// OutputOptions defines the set of options for writing dump data.
type OutputOptions struct {
	Out                        string   `long:"out" value-name:"<directory-path>" short:"o" description:"output directory, or '-' for stdout (default: 'dump')"`
	Gzip                       bool     `long:"gzip" description:"compress archive our collection output with Gzip"`
	Oplog                      bool     `long:"oplog" description:"use oplog for taking a point-in-time snapshot"`
	Archive                    string   `long:"archive" value-name:"<file-path>" optional:"true" optional-value:"-" description:"dump as an archive to the specified path. If flag is specified without a value, archive is written to stdout"`
	DumpDBUsersAndRoles        bool     `long:"dumpDbUsersAndRoles" description:"dump user and role definitions for the specified database"`
	ExcludedCollections        []string `long:"excludeCollection" value-name:"<collection-name>" description:"collection to exclude from the dump (may be specified multiple times to exclude additional collections)"`
	ExcludedCollectionPrefixes []string `long:"excludeCollectionsWithPrefix" value-name:"<collection-prefix>" description:"exclude all collections from the dump that have the given prefix (may be specified multiple times to exclude additional prefixes)"`
	NumParallelCollections     int      `long:"numParallelCollections" short:"j" description:"number of collections to dump in parallel" default:"4" default-mask:"-"`
	ViewsAsCollections         bool     `long:"viewsAsCollections" description:"dump views as normal collections with their produced data, omitting standard collections"`
}

// Name returns a human-readable group name for output options.
func (*OutputOptions) Name() string {
	return "output"
}
