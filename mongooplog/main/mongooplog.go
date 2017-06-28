// Main package for the mongooplog tool.
package main

import (
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongooplog"
	"os"
)

func main() {
	// initialize command line options
	opts := options.New("mongooplog", mongooplog.Usage,
		options.EnabledOptions{Auth: true, Connection: true, Namespace: false, URI: true})

	// add the mongooplog-specific options
	sourceOpts := &mongooplog.SourceOptions{}
	opts.AddOptions(sourceOpts)

	log.Logvf(log.Always, "warning: mongooplog is deprecated, and will be removed completely in a future release")

	// parse the command line options
	args, err := opts.ParseArgs(os.Args[1:])
	if err != nil {
		log.Logvf(log.Always, "error parsing command line options: %v", err)
		log.Logvf(log.Always, "try 'mongooplog --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	if len(args) != 0 {
		log.Logvf(log.Always, "positional arguments not allowed: %v", args)
		log.Logvf(log.Always, "try 'mongooplog --help' for more information")
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

	// init logger
	log.SetVerbosity(opts.Verbosity)
	signals.Handle()

	// verify uri options and log them
	opts.URI.LogUnsupportedOptions()

	// validate the mongooplog options
	if sourceOpts.From == "" {
		log.Logvf(log.Always, "command line error: need to specify --from")
		os.Exit(util.ExitBadOptions)
	}

	// create a session provider for the destination server
	sessionProviderTo, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logvf(log.Always, "error connecting to destination host: %v", err)
		os.Exit(util.ExitError)
	}
	defer sessionProviderTo.Close()

	// create a session provider for the source server
	opts.Connection.Host = sourceOpts.From
	opts.Connection.Port = ""
	sessionProviderFrom, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logvf(log.Always, "error connecting to source host: %v", err)
		os.Exit(util.ExitError)
	}
	defer sessionProviderFrom.Close()

	// initialize mongooplog
	oplog := mongooplog.MongoOplog{
		ToolOptions:         opts,
		SourceOptions:       sourceOpts,
		SessionProviderFrom: sessionProviderFrom,
		SessionProviderTo:   sessionProviderTo,
	}

	// kick it off
	if err := oplog.Run(); err != nil {
		log.Logvf(log.Always, "error: %v", err)
		os.Exit(util.ExitError)
	}

}
