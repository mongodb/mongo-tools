// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongoexport tool.
package main

import (
	"os"
	"time"

	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/progress"
	"github.com/mongodb/mongo-tools-common/signals"
	"github.com/mongodb/mongo-tools-common/util"
	"github.com/mongodb/mongo-tools/mongoexport"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	progressBarLength   = 24
	progressBarWaitTime = time.Second
)

func main() {
	// initialize command-line opts
	opts := options.New("mongoexport", mongoexport.Usage,
		options.EnabledOptions{Auth: true, Connection: true, Namespace: true, URI: true})

	outputOpts := &mongoexport.OutputFormatOptions{}
	opts.AddOptions(outputOpts)
	inputOpts := &mongoexport.InputOptions{}
	opts.AddOptions(inputOpts)

	args, err := opts.ParseArgs(os.Args[1:])
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %v", err)
		log.Logvf(log.Always, "try 'mongoexport --help' for more information")
		os.Exit(util.ExitBadOptions)
	}
	if len(args) != 0 {
		log.Logvf(log.Always, "too many positional arguments: %v", args)
		log.Logvf(log.Always, "try 'mongoexport --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	log.SetVerbosity(opts.Verbosity)
	signals.Handle()

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	if inputOpts.SlaveOk {
		if inputOpts.ReadPreference != "" {
			log.Logvf(log.Always, "--slaveOk can't be specified when --readPreference is specified")
			os.Exit(util.ExitBadOptions)
		}
		log.Logvf(log.Always, "--slaveOk is deprecated and being internally rewritten as --readPreference=nearest")
		inputOpts.ReadPreference = "nearest"
	}

	if inputOpts.ReadPreference != "" {
		pref, err := db.ParseReadPreference(inputOpts.ReadPreference)
		if err != nil {
			log.Logvf(log.Always, "error parsing --ReadPreference: %v", err)
			os.Exit(util.ExitBadOptions)
		}

		opts.ReadPreference = pref
	}

	provider, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logvf(log.Always, "%v", err)
		os.Exit(util.ExitError)
	}
	defer provider.Close()

	isMongos, err := provider.IsMongos()
	if err != nil {
		log.Logvf(log.Always, "%v", err)
		os.Exit(util.ExitError)
	}

	// warn if we are trying to export from a secondary in a sharded cluster
	if isMongos && opts.ReadPreference != nil && opts.ReadPreference.Mode() != readpref.PrimaryMode {
		log.Logvf(log.Always, db.WarningNonPrimaryMongosConnection)
	}

	progressManager := progress.NewBarWriter(log.Writer(0), progressBarWaitTime, progressBarLength, false)
	progressManager.Start()
	defer progressManager.Stop()

	exporter := mongoexport.MongoExport{
		ToolOptions:     *opts,
		OutputOpts:      outputOpts,
		InputOpts:       inputOpts,
		SessionProvider: provider,
		ProgressManager: progressManager,
	}

	err = exporter.ValidateSettings()
	if err != nil {
		log.Logvf(log.Always, "error validating settings: %v", err)
		log.Logvf(log.Always, "try 'mongoexport --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	writer, err := exporter.GetOutputWriter()
	if err != nil {
		log.Logvf(log.Always, "error opening output stream: %v", err)
		os.Exit(util.ExitError)
	}
	if writer == nil {
		writer = os.Stdout
	} else {
		defer writer.Close()
	}

	numDocs, err := exporter.Export(writer)
	if err != nil {
		log.Logvf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}

	if numDocs == 1 {
		log.Logvf(log.Always, "exported %v record", numDocs)
	} else {
		log.Logvf(log.Always, "exported %v records", numDocs)
	}

}
