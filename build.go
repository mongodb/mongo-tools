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
	taskRegistry.Declare("build").
		Description("build the tools").
		OptionalArgs("pkgs").
		Do(buildscript.BuildTools)
	taskRegistry.Declare("checkMinVersion").
		Description("check if the minimum required Go version exists").
		Do(buildscript.CheckMinimumGoVersion)

	// Deps & SSDL
	taskRegistry.Declare("writeSBOMLite").
		Description("create an SBOM Lite file using the Silkbomb tool").
		Do(buildscript.WriteSBOMLite)
	taskRegistry.Declare("writeAugmentedSBOM").
		Description("create an Augmented SBOM file using the Silkbomb tool").
		Do(buildscript.WriteAugmentedSBOM)
	taskRegistry.Declare("addDep").
		Description("Add a dependency").
		RequiredArg("pkg").
		Do(buildscript.AddDep)
	taskRegistry.Declare("updateDep").
		Description("Update a dependency").
		RequiredArg("pkg").
		Do(buildscript.UpdateDep)
	taskRegistry.Declare("updateAllDeps").
		Description("Update all dependencies").
		OptionalArg("exclude").
		Do(buildscript.UpdateAllDeps)
	taskRegistry.Declare("writeThirdPartyNotices").
		Description("Write the THIRD-PARTY-NOTICES file").
		Do(buildscript.WriteThirdPartyNotices)

	// Static Analysis
	taskRegistry.Declare("sa:installdevtools").
		Description("installs dev tools").
		Do(buildscript.SAInstallDevTools)
	taskRegistry.Declare("sa:lint").
		Description("runs precious linting").
		DependsOn("sa:installdevtools").
		Do(buildscript.SAPreciousLint)
	taskRegistry.Declare("sa:modtidy").Description("runs go mod tidy").Do(buildscript.SAModTidy)
	taskRegistry.Declare("sa:evgvalidate").
		Description("runs evergreen validate").
		Do(buildscript.SAEvergreenValidate)

	// Testing
	taskRegistry.Declare("test:unit").
		Description("runs all unit tests").
		OptionalArgs("pkgs", "race").
		Do(buildscript.TestUnit)
	taskRegistry.Declare("test:integration").
		Description("runs all integration tests").
		OptionalArgs("pkgs", "ssl", "auth", "kerberos", "topology", "race").
		Do(buildscript.TestIntegration)
	taskRegistry.Declare("test:kerberos").
		Description("runs all kerberos tests").
		OptionalArgs("race").
		Do(buildscript.TestKerberos)
	taskRegistry.Declare("test:awsauth").
		Description("runs all aws auth tests").
		OptionalArgs("race").
		Do(buildscript.TestAWSAuth)
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
