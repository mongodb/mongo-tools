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
	taskRegistry.Declare("build").Description("build the tools").OptionalArgs("tools").Do(buildscript.BuildTools)
	taskRegistry.Declare("test:unit").Description("runs unit tests").OptionalArgs("tools").Do(buildscript.TestUnit)
	taskRegistry.Declare("test:integration").Description("runs integration tests").OptionalArgs("tools", "ssl", "auth", "kerberos", "topology").Do(buildscript.TestIntegration)
	taskRegistry.Declare("test:kerberos").Description("runs kerberos tests").Do(buildscript.TestKerberos)
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
