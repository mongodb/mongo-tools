// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Main package for the mongofiles tool.
package main

import (
	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/signals"
	"github.com/mongodb/mongo-tools-common/util"
	"github.com/mongodb/mongo-tools/mongofiles"

	"fmt"
	"os"
)

func main() {
	storageOpts := &mongofiles.StorageOptions{}
	inputOpts := &mongofiles.InputOptions{}
	args, opts, err := mongofiles.ParseOptions(os.Args[1:], storageOpts, inputOpts)
	if err != nil {
		log.Logv(log.Always, err.Error())
		os.Exit(util.ExitBadOptions)
	}

	signals.Handle()

	// print help, if specified
	if opts.PrintHelp(false) {
		os.Exit(util.ExitClean)
	}

	// print version, if specified
	if opts.PrintVersion() {
		os.Exit(util.ExitClean)
	}

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
