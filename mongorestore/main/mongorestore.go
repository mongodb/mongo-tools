package main

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	commonopts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/mongodb/mongo-tools/mongorestore/options"
	"os"
)

func main() {
	// initialize command-line opts
	opts := commonopts.New("mongorestore", "<options>", commonopts.EnabledOptions{Auth: true, Connection: true, Namespace: true})
	inputOpts := &options.InputOptions{}
	opts.AddOptions(inputOpts)
	outputOpts := &options.OutputOptions{}
	opts.AddOptions(outputOpts)

	extraArgs, err := opts.Parse()
	if err != nil {
		fmt.Printf("error parsing command line options: %v\n\n", err)
		fmt.Printf("try 'mongorestore --help' for more information\n")
		os.Exit(util.ExitBadOptions)
	}

	// print help or version info, if specified
	if opts.PrintHelp(false) {
		return
	}
	if opts.PrintVersion() {
		return
	}

	log.SetVerbosity(opts.Verbosity)

	targetDir, err := getTargetDirFromArgs(extraArgs, inputOpts.Directory)
	if err != nil {
		fmt.Printf("error parsing command line options: %v\n", err)
		os.Exit(util.ExitBadOptions)
	}
	targetDir = util.ToUniversalPath(targetDir)

	// connect directly, unless a replica set name is explicitly specified
	_, setName := util.ParseConnectionString(opts.Host)
	opts.Direct = (setName == "")

	provider, err := db.NewSessionProvider(*opts)
	if err != nil {
		log.Logf(log.Always, "error connecting to host: %v\n", err)
		os.Exit(util.ExitError)
	}
	restore := mongorestore.MongoRestore{
		ToolOptions:     opts,
		OutputOptions:   outputOpts,
		InputOptions:    inputOpts,
		TargetDirectory: targetDir,
		SessionProvider: provider,
	}

	err = restore.Restore()
	if err != nil {
		log.Logf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}
}

// getTargetDirFromArgs handles the logic and error cases of figuring out
// the target restore directory.
func getTargetDirFromArgs(extraArgs []string, dirFlag string) (string, error) {
	// This logic is in a switch statement so that the rules are understandable.
	// We start by handling error cases, and then handle the different ways the target
	// directory can be legally set.
	switch {
	case len(extraArgs) > 1:
		// error on cases when there are too many positional arguments
		return "", fmt.Errorf("too many positional arguments")

	case dirFlag != "" && len(extraArgs) > 0:
		// error when positional arguments and --dir are used
		return "", fmt.Errorf(
			"cannot use both --dir and a positional argument to set the target directory")

	case len(extraArgs) == 1:
		// a nice, simple case where one argument is given, so we use it
		return extraArgs[0], nil

	case dirFlag != "":
		// if we have no extra args and a --dir flag, use the --dir flag
		log.Log(log.Info, "using --dir flag instead of arguments")
		return dirFlag, nil

	default:
		log.Log(log.Info, "using default 'dump' directory")
		return "dump", nil
	}
}
