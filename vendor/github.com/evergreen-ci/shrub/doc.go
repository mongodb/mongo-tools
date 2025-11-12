// Package shrub provides a simple, low-overhead interface for
// generating Evergreen project configurations.
//
// For most use cases, you can start with a Configuration struct and add data
// such as new tasks, task groups, build variants, and functions to that
// Configuration using the provided setters. For example:
//
//	// Create an empty configuration that will be populated later.
//	conf := &Configuration{}
//
//	// The following function definitions is equivalent to the YAML
//	// configuration:
//	// functions:
//	//   - name: my-new-func
//	//     commands:
//	//       - name: git.get_project
//	//         params:
//	//           directory: my-working-directory
//	//       - name: shell.exec
//	//         params:
//	//           script: echo hello world!
//	newFunc := conf.Function("my-new-func")
//	newFunc.Command().Command("git.get_project").Param("directory", "my-working-directory")
//	newFunc.Command().Command("shell.exec").Param("script", "echo hello world!")
//
//	// The following task definition is equivalent to the YAML configuration:
//	// tasks:
//	//   - name: my-new-task
//	//     commands:
//	//        - func: my-new-func
//	newTask := conf.Task("my-new-task")
//	newTask.Function("my-new-func")
//
//	// The following build variant definitions is equivalent to the YAML
//	// configuration:
//	// buildvariants:
//	//   - name: my-new-build-variant
//	//     run_on:
//	//       - some-distro
//	//     tasks:
//	//       - name: my-new-task
//	newBV := conf.Variant("my-new-build-variant")
//	newBV.RunOn("some-distro")
//	newBV.AddTasks("my-new-task")
//
//	// Marshal to JSON so it can be used for generate.tasks.
//	res, err := json.MarshalIndent(conf, "", "    ")
//	fmt.Println(res)
//
// Be aware that some command methods will panic if you attempt to
// construct an invalid command. You can wrap your configuration logic with
// BuildConfiguration to convert any panic into an error.
package shrub
