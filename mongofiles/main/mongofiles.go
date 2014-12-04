package main

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	commonopts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongofiles"
	"github.com/mongodb/mongo-tools/mongofiles/options"
	"os"
)

const (
	Usage = `[options] command [gridfs filename]
        command:
          one of (list|search|put|get|delete)
          list - list all files.  'gridfs filename' is an optional prefix
                 which listed filenames must begin with.
          search - search all files. 'gridfs filename' is a substring
                   which listed filenames must contain.
          put - add a file with filename 'gridfs filename'
          get - get a file with filename 'gridfs filename'
          delete - delete all files with filename 'gridfs filename'
        `
)

func main() {
	// initialize command-line opts
	opts := commonopts.New("mongofiles", Usage, commonopts.EnabledOptions{Auth: true, Connection: true, Namespace: false})

	storageOpts := &options.StorageOptions{}
	opts.AddOptions(storageOpts)

	args, err := opts.Parse()
	if err != nil {
		log.Logf(log.Always, "error parsing command line options: %v", err)
		opts.PrintHelp(true)
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

	// add the specified database to the namespace options struct
	opts.Namespace.DB = storageOpts.DB

	// connect directly, unless a replica set name is explicitly specified
	_, setName := util.ParseConnectionString(opts.Host)
	opts.Direct = (setName == "")

	// create a session provider to connect to the db
	provider, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logf(log.Always, "error connecting to host: %v\n", err)
		os.Exit(util.ExitError)
	}
	mf := mongofiles.MongoFiles{
		ToolOptions:     opts,
		StorageOptions:  storageOpts,
		SessionProvider: provider,
	}

	if err := mf.ValidateCommand(args); err != nil {
		log.Logf(log.Always, "error: %v", err)
		opts.PrintHelp(true)
		os.Exit(util.ExitError)
	}

	output, err := mf.Run(true)
	if err != nil {
		log.Logf(log.Always, "%v", err)
		os.Exit(util.ExitError)
	}
	fmt.Printf("%s", output)
}
