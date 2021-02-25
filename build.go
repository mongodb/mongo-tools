package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/craiggwilson/goke/task"
	"github.com/mongodb/mongo-tools/buildscript"
)

var taskRegistry = task.NewRegistry(task.WithAutoNamespaces(true))

func init() {
	// Build
	taskRegistry.Declare("build").Description("build the tools").OptionalArgs("tools").Do(buildscript.BuildTools)

	// Static Analysis
	taskRegistry.Declare("sa:modtidy").Description("runs go mod tidy").Do(buildscript.SAModTidy)

	// Tools Testing
	taskRegistry.Declare("test:tools.unit").Description("runs tools unit tests").OptionalArgs("tools").Do(buildscript.TestToolsUnit)
	taskRegistry.Declare("test:tools.integration").Description("runs tools integration tests").OptionalArgs("tools", "ssl", "auth", "kerberos", "topology").Do(buildscript.TestToolsIntegration)
	taskRegistry.Declare("test:tools.kerberos").Description("runs tools kerberos tests").Do(buildscript.TestToolsKerberos)

	// Tools Common Testing
	taskRegistry.Declare("test:common.unit").Description("runs common unit tests").OptionalArgs("tools").Do(buildscript.TestCommonUnit)
}

func main() {
	err := task.Run(taskRegistry, os.Args[1:])
	if err == flag.ErrHelp {
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
