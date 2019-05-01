// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoexport

import (
	"fmt"
	"io/ioutil"

	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
)

var Usage = `<options>

Export data from MongoDB in CSV or JSON format.

See http://docs.mongodb.org/manual/reference/program/mongoexport/ for more information.`

// OutputFormatOptions defines the set of options to use in formatting exported data.
type OutputFormatOptions struct {
	// Fields is an option to directly specify comma-separated fields to export to CSV.
	Fields string `long:"fields" value-name:"<field>[,<field>]*" short:"f" description:"comma separated list of field names (required for exporting CSV) e.g. -f \"name,age\" "`

	// FieldFile is a filename that refers to a list of fields to export, 1 per line.
	FieldFile string `long:"fieldFile" value-name:"<filename>" description:"file with field names - 1 per line"`

	// Type selects the type of output to export as (json or csv).
	Type string `long:"type" value-name:"<type>" default:"json" default-mask:"-" description:"the output format, either json or csv (defaults to 'json')"`

	// Deprecated: allow legacy --csv option in place of --type=csv
	CSVOutputType bool `long:"csv" default:"false" hidden:"true"`

	// OutputFile specifies an output file path.
	OutputFile string `long:"out" value-name:"<filename>" short:"o" description:"output file; if not specified, stdout is used"`

	// JSONArray if set will export the documents an array of JSON documents.
	JSONArray bool `long:"jsonArray" description:"output to a JSON array rather than one object per line"`

	// Pretty displays JSON data in a human-readable form.
	Pretty bool `long:"pretty" description:"output JSON formatted to be human-readable"`

	// NoHeaderLine, if set, will export CSV data without a list of field names at the first line.
	NoHeaderLine bool `long:"noHeaderLine" description:"export CSV data without a list of field names at the first line"`

	// JSONFormat specifies what extended JSON format to export (canonical or relaxed). Defaults to relaxed.
	JSONFormat jsonFormat `long:"jsonFormat" value-name:"<type>" default:"relaxed" description:"the extended JSON format to output, either canonical or relaxed (defaults to 'relaxed')"`
}

// Name returns a human-readable group name for output format options.
func (*OutputFormatOptions) Name() string {
	return "output"
}

// InputOptions defines the set of options to use in retrieving data from the server.
type InputOptions struct {
	Query          string `long:"query" value-name:"<json>" short:"q" description:"query filter, as a JSON string, e.g., '{x:{$gt:1}}'"`
	QueryFile      string `long:"queryFile" value-name:"<filename>" description:"path to a file containing a query filter (JSON)"`
	SlaveOk        bool   `long:"slaveOk" short:"k" description:"allow secondary reads if available (default true)" default:"false" default-mask:"-"`
	ReadPreference string `long:"readPreference" value-name:"<string>|<json>" description:"specify either a preference mode (e.g. 'nearest') or a preference json object (e.g. '{mode: \"nearest\", tagSets: [{a: \"b\"}], maxStalenessSeconds: 123}')"`
	ForceTableScan bool   `long:"forceTableScan" description:"force a table scan (do not use $snapshot)"`
	Skip           int64  `long:"skip" value-name:"<count>" description:"number of documents to skip"`
	Limit          int64  `long:"limit" value-name:"<count>" description:"limit the number of documents to export"`
	Sort           string `long:"sort" value-name:"<json>" description:"sort order, as a JSON string, e.g. '{x:1}'"`
	AssertExists   bool   `long:"assertExists" default:"false" description:"if specified, export fails if the collection does not exist"`
}

// Name returns a human-readable group name for input options.
func (*InputOptions) Name() string {
	return "querying"
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

// Options represents all possible options that can be used to configure mongoexport.
type Options struct {
	*options.ToolOptions
	*OutputFormatOptions
	*InputOptions
	ParsedArgs []string
}

// ParseOptions reads command line arguments and converts them into options that can be used to configure mongoexport.
func ParseOptions(rawArgs []string, versionStr, gitCommit string) (Options, error) {
	// initialize command-line opts
	opts := options.New("mongoexport", versionStr, gitCommit, Usage,
		options.EnabledOptions{Auth: true, Connection: true, Namespace: true, URI: true})
	outputOpts := &OutputFormatOptions{}
	opts.AddOptions(outputOpts)
	inputOpts := &InputOptions{}
	opts.AddOptions(inputOpts)
	opts.AddKnownURIParameters(options.KnownURIOptionsReadPreference)

	args, err := opts.ParseArgs(rawArgs)
	if err != nil {
		return Options{}, err
	}
	if len(args) != 0 {
		return Options{}, fmt.Errorf("too many positional arguments: %v", args)
	}

	log.SetVerbosity(opts.Verbosity)

	// verify URI options and log them
	opts.URI.LogUnsupportedOptions()

	if inputOpts.SlaveOk {
		if inputOpts.ReadPreference != "" {
			return Options{}, fmt.Errorf("--slaveOk can't be specified when --readPreference is specified")
		}

		log.Logvf(log.Always, "--slaveOk is deprecated and --readPreference=nearest should be used instead")
		inputOpts.ReadPreference = "nearest"
	}

	opts.ReadPreference, err = db.NewReadPreference(inputOpts.ReadPreference, opts.URI.ParsedConnString())
	if err != nil {
		return Options{}, fmt.Errorf("error parsing --readPreference: %v", err)
	}

	return Options{
		opts,
		outputOpts,
		inputOpts,
		args,
	}, nil
}
