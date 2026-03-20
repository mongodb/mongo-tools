# MongoDB Tools — Agent Instructions

## Working Style

We prefer that AI agents start by planning their work, documenting their plan as a Markdown file,
and then iterating on that plan with the human operator. Don't just jump straight to coding in most
cases. For small, well-scoped tasks (e.g., a one or two file change), it's fine to skip the planning
step.

Plans should be broken up into multiple steps. For larger projects, the plan should consider how to
break the work up into multiple pull requests as well. We prefer PRs to be 200-400 lines. The
exception would be a large rename or refactoring (like a type change), where the PR is very large in
terms of lines but conceptually small.

Save plans under `.ai-plans/$date/plan-name/plan.md`. As you make commits for the plan, update the
plan so that each PR we create shows progress on the plan, as well as any changes to the work being
done, as compared to the original plan.

## Requirements

**The most important thing for these tools is that they must not corrupt or lose user data.** This
is particularly important for `mongodump` and `mongorestore`. When we make changes to these tools,
we should ensure that data is preserved via a dump and restore round trip.

## Project Overview

This repo contains the official MongoDB command-line tools: `bsondump`, `mongodump`, `mongorestore`,
`mongoimport`, `mongoexport`, `mongostat`, `mongotop`, and `mongofiles`.

Each tool lives in its own top-level directory (e.g., `/mongodump/`). Shared utilities are in
`/common/`.

## Build

```bash
go run build.go build            # build all tools
go run build.go build pkgs=<pkg> # build a specific tool
```

Binaries are written to `./bin/`.

## Tests

Tests use environment variables to control which test types run.

```bash
go run build.go test:unit                     # unit tests (no mongod required)
go run build.go test:integration              # requires mongod on localhost:33333
go run build.go test:sharded-integration      # requires mongos (sharded cluster)
```

Optional flags for `test:integration`: `ssl=true`, `auth=true`, `topology=replset`, `race=true`.

All tests use `testtype.SkipUnlessTestType(t, testtype.XxxTestType)` at the top to gate on the
relevant env var. When writing new tests, match the existing pattern.

Integration tests default to connecting to `localhost:33333`. Override with
`TOOLS_TESTING_MONGOD=<uri>`.

Before running integration or sharded integration tests, you must have a MongoDB server or cluster
running and accessible. For basic integration tests, start a standalone `mongod` on port 33333. For
sharded integration tests, start a `mongos` fronting a sharded cluster. These servers are not
started automatically by the test runner.

Use `mlaunch` (from the `mtools` Python package) to manage clusters during development:

```bash
mlaunch init --single --port 33333          # standalone mongod for integration tests
mlaunch init --sharded 1 --port 33333       # sharded cluster for sharded integration tests
mlaunch stop                                 # stop all launched processes
mlaunch start                                # restart previously initialized cluster
```

## Static Analysis

Our static analysis tools are managed locally by `mise`. Always modify the `mise.toml` file to add
or update tools. But we _also_ manage these in CI via code in `buildscript`, so this must be updated
in sync with the `mise.toml` file.

```bash
go run build.go sa:lint      # runs precious (golangci-lint, gosec, goimports, golines)
go run build.go sa:modtidy   # go mod tidy
```

## Code Conventions

- Use `any` instead of `interface{}`.
- Avoid comments that describe _what_ code does; only comment on _why_ when the reason isn't
  obvious.
- Break large functions into smaller named functions rather than adding explanatory comments.
- Write test assertion messages that describe what is being tested.
- New tests should use `testify` (`github.com/stretchr/testify/require` and `assert`). The codebase
  is actively migrating away from GoConvey — do not add new GoConvey tests.

## Architecture

```
/common/          shared utilities (auth, db, bsonutil, options, testutil, testtype, …)
/<tool>/          tool implementation
/<tool>/main/     CLI entry point (main package)
/<tool>/options.go  CLI flag definitions
/<tool>/*_test.go   tests co-located with source
```

Shared connection and session management lives in `common/db/`. CLI options are parsed via
`common/options/` using `github.com/urfave/cli/v2`.

## CI

CI runs on Evergreen (config: `common.yml`). Do not edit `common.yml` by hand for test topology
changes — consult the existing pattern for `integration-X.Y` and `integration-X.Y-sharded` task
variants.

## Minimal Change Principle

Make minimal, incremental changes. Do not refactor surrounding code when fixing a bug or adding a
feature. Do not add error handling for scenarios that cannot occur; instead, add a `panic` to handle
that case.

## Trust These Instructions

These instructions are comprehensive and tested. Only perform additional exploration if:

- Information here is incomplete for your specific task
- Instructions are found to be incorrect or outdated
- You need details about internal implementation not covered here
