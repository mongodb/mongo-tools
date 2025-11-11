# Evergreen task generator

This program generates JSON files that are meant to be handed to evergreen's `generate.tasks`
command. The actual JSON files are generated using
[shrub](https://pkg.go.dev/github.com/evergreen-ci/shrub), which is maintained by the Evergreen
team. Everything else here is abstraction to make those easier to write.

## Architecture

main.go is extremely small; it initializes the other packages for MongodumpTaskGen mode, then calls
the app's Run method. The code is almost all shared with the (mongosync) task-generator, with small
differences in behavior controlled by IsMongodumpTaskGen().

`package cli` is just an [urfave cli](https://cli.urfave.org/), which we already use in a number of
places. It has subcommands for each kind of task we generate (FSM, Fuzzer, Passthrough, Upgrade),
and if run with no subcommand, it generates everything.

`package generate` is the heart of the generator. The `Spec` struct is basically a wrapper around
the CLI options, describing what sort of tasks we should actually generate. The bulk of the logic is
in `generator` struct, which calls the various methods that generate the actual tasks.

The thing you're most likely to touch, if you're not adding a new kind of generator, are the big
lists of tasks, in `passthrough_tasks.go`, `fuzz_tasks.go`, and `fsm_tasks.go`. Here's an example
from the passthrough tasks; all the rest are similar:

```go
passthrough("ctc_rs_sc_nsfilter_jscore_passthrough").
    timeoutMinutes(45).
    maxJobs(8).
    skipForSrcVersions(v50, v44),
```

This is a standard builder pattern, though there's no actual `Build` method. The `passthrough`
function takes the name of the task, and all other methods are optional. If provided, they'll set
the field that actually gets generated in the resulting JSON file; if not, they'll pick up
reasonable defaults.

## Running manual patches

One byproduct of this work means that running manual patches with `evergreen patch` is slightly more
complicated than it was before. Because there are no tasks actually named (for example)
`ctc_rs_sc_nsfilter_jscore_passthrough` in our evergreen.yml file, running
`evergreen patch -t ctc_rs_sc_nsfilter_jscore_passthrough` will not work (or rather: it will appear
to work, but it will run 0 tasks).

To facilitate manual patch runs, there is a task in evergreen.yml, `run_manual_generated_tasks`. To
run this task, do something like the following:

```
go run ./cmd/task-generator passthrough --version 4.4 --buildvariants > evergreen/manual-tasks.json

evergreen patch -v rhel80-generated -t run_manual_generated_tasks
```

This task always uses the contents of `evergreen/manual-tasks.json`. The committed version of this
file is just an empty JSON object, so it's effectively a no-op in CI. Changes to this file should
not be merged to main; it exists only to facilitate manual patch runs.

### Using the helper for passthrough suites

Please note that `run-manual-patch` does not work for non-passthrough tests. If, for example, you'd
like to patch some e2e tests you can run:

```
go run ./cmd/task-generator e2e --variant ubuntu1804 > evergreen/manual-tasks.json
evergreen patch -v ubuntu1804 -t generate_e2e_tasks
```

There is also a helper program, `evergreen/run-manual-patch`, which first runs the generator and
then the correct `evergreen patch` command.

```
 $ ./evergreen/run-manual-patch -h
usage: run-manual-patch [-h] --type TYPE [--task TASK] [--version X.Y] [--uncommitted]

Run the task generator, then run evergreen patch with the results

options:
  -h, --help         show this help message and exit
  --type TYPE        type of suite to generate (passthrough, fsm, etc.)
  --task TASK        regex of task name to generate (default: all)
  --version X.Y      mongo versions to run tests on (default: all)
  --uncommitted, -u  include uncommitted changes

All other arguments are passed through to 'evergreen patch'.
```

For example, if you want to run all the passthrough suites on mongo version 4.4, you could run:

```
$ ./evergreen/run-manual-patch --type passthrough --version 4.4 -u -y -f
```

The `--type` argument is required, and everything else is optional. Arguments that are not
recognized by the wrapper program are passed through to `evergreen patch`, so you can also give this
patch a `--description` or something if you like.

Good luck!
