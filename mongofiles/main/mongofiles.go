// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongofiles tool.
package main

import (
	"github.com/mongodb/mongo-go-driver/mongo/readpref"
	"github.com/mongodb/mongo-go-driver/mongo/writeconcern"
	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/signals"
	"github.com/mongodb/mongo-tools-common/util"
	"github.com/mongodb/mongo-tools/mongofiles"

	"fmt"
	"os"
)

func main() {
	// initialize command-line opts
	opts := options.New("mongofiles", mongofiles.Usage, options.EnabledOptions{Auth: true, Connection: true, Namespace: false, URI: true})

	storageOpts := &mongofiles.StorageOptions{}
	opts.AddOptions(storageOpts)
	inputOpts := &mongofiles.InputOptions{}
	opts.AddOptions(inputOpts)
	opts.URI.AddKnownURIParameters(options.KnownURIOptionsReadPreference)

	args, err := opts.ParseArgs(os.Args[1:])
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %v", err)
		log.Logvf(log.Always, "try 'mongofiles --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}
	log.SetVerbosity(opts.Verbosity)
	signals.Handle()

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	// add the specified database to the namespace options struct
	opts.Namespace.DB = storageOpts.DB

	// set WriteConcern
	var writeConcern *writeconcern.WriteConcern
	if storageOpts.WriteConcern != "" {
		writeConcern, err = db.PackageWriteConcernOptsToObject(storageOpts.WriteConcern, nil)
	} else if opts.ConnectionString != "" {
		writeConcern, err = db.PackageWriteConcernOptsToObject("", opts.ParsedConnString())
	}
	if err != nil {
		return
	}
	opts.WriteConcern = writeConcern

	// set ReadPreference
	var readPref *readpref.ReadPref
	if inputOpts.ReadPreference != "" {
		readPref, err = db.ParseReadPreference(inputOpts.ReadPreference)
		if err != nil {
			log.Logv(log.Always, fmt.Sprintf("error parsing --readPreference : %v", err))
			return
		}
	} else {
		readPref = readpref.Nearest()
	}
	opts.ReadPreference = readPref

	// create a session provider to connect to the db
	provider, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logvf(log.Always, "error connecting to host: %v", err)
		os.Exit(util.ExitError)
	}
	defer provider.Close()
	mf := mongofiles.MongoFiles{
		ToolOptions:     opts,
		StorageOptions:  storageOpts,
		SessionProvider: provider,
		InputOptions:    inputOpts,
	}

	if err := mf.ValidateCommand(args); err != nil {
		log.Logvf(log.Always, "%v", err)
		log.Logvf(log.Always, "try 'mongofiles --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	output, err := mf.Run(true)
	if err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}
	fmt.Printf("%s", output)
}
