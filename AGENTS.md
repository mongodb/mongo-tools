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

When you create a plan, include this in the PR as a comment with an attachment. If the user creates
the PR themselves, remind them to include this.

## Requirements

**The most important thing for these tools is that they must not corrupt or lose user data.** This
is particularly important for `mongodump` and `mongorestore`. When we make changes to these tools,
we should ensure that data is preserved via a dump and restore round trip.

## Toolchain (`mise`)

All dev tools — **including `go` itself** — are managed by `mise`, so a bare `go` may not exist or
may be the wrong version. If you have `mise` shell integration, the commands in this file work
as-written. Otherwise prefix them:

```bash
mise exec go -- go run build.go test:unit
```

Always list the tools `mise exec` needs. Without that list it tries to install _every_ tool in
`mise.toml`, which fails on some CI platforms. See `CONTRIBUTING.md` for setup.

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
go run build.go test:kerberos                 # requires Kerberos-enabled server
go run build.go test:awsauth                  # requires AWS auth setup
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

A test that writes directly to `local.oplog.rs` needs a **standalone** `mongod` — those writes are
rejected on a replica set member. Note that Evergreen's `integration-*-cluster` variants are replica
sets, so such a test can pass locally against a standalone and fail in CI.

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
- New tests should use `testify` (`github.com/stretchr/testify/require` and `assert`). Prefer
  `EqualValues` over casting an operand just to satisfy `Equal`.

## Linting

Run `go run build.go sa:lint` to check for lint errors. Ignore LSP diagnostics that are suppressed
by `.golangci.yml` — in particular, `bson.E struct literal uses unkeyed fields` is explicitly
allowed and should not be treated as an error.

## Architecture

Each tool lives in its own top-level directory with its CLI entry point under `<tool>/main/`, and
tests are co-located with the source. Shared connection and session management lives in
`common/db/`. CLI options are parsed via `common/options/` using `github.com/urfave/cli/v2`.

## Dependencies

**Never use `go get` to add or update a dependency, and never hand-edit anything under `vendor/`.**
Dependencies are vendored, and a dependency change must also update `go.{mod,sum}`, the SBOM Lite
file (`cyclonedx.sbom.json`), and `THIRD-PARTY-NOTICES`. `go get` updates none of those. Use:

```bash
go run build.go addDep -pkg=github.com/some/package@v1.2.3
go run build.go updateDep -pkg=github.com/some/package
```

These require Podman. See `CONTRIBUTING.md` for the full flow.

## Git and Pull Requests

- **Commit messages are prefixed with the Jira ticket ID**, e.g.
  `TOOLS-4275 Remove pointless stdlib wrappers in util package`. Tickets live in the `TOOLS` Jira
  project. Ask the human for the ticket ID if you don't have one — don't invent one.
- Install the pre-commit hook once with `git-hooks/setup`. It runs `precious lint --staged` and will
  block a commit that fails lint.
- Tidy and lint before committing, so the hook doesn't fail on you:

```bash
mise exec github:houseabsolute/precious -- precious tidy -g
mise exec github:houseabsolute/precious -- precious lint -g
```

- Do not commit or push without the human's go-ahead. Default to staging the work and drafting a
  commit message for review.

## CI

CI runs on Evergreen (config: `common.yml`). Do not edit `common.yml` by hand for test topology
changes — consult the existing pattern for `integration-X.Y` and `integration-X.Y-sharded` task
variants.

## Minimal Change Principle

Make minimal, incremental changes. Do not refactor surrounding code when fixing a bug or adding a
feature. Do not add error handling for scenarios that cannot occur; instead, add a `panic` to handle
that case.

## Code in `common` is Used in Other Projects

The code in `common` is used by other projects at MongoDB. If you want to modify public APIs in that
package, double check with the human directing you before doing this.

## Trust These Instructions

These instructions are comprehensive and tested. Only perform additional exploration if:

- Information here is incomplete for your specific task
- Instructions are found to be incorrect or outdated
- You need details about internal implementation not covered here
