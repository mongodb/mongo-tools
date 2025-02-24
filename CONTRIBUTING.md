# Contributing to the MongoDB Tools Project

Pull requests are always welcome, and the MongoDB engineering team appreciates any help the
community can give to make the MongoDB tools better.

For any particular improvement you want to make, you can begin a discussion on the
[MongoDB Developers Forum](https://groups.google.com/forum/?fromgroups#!forum/mongodb-dev). This is
the best place to discuss your proposed improvement (and its implementation) with the core
development team.

If you're interested in contributing, we have a list of some suggested tickets that are easy enough
to get started on
[here](https://jira.mongodb.org/issues/?jql=project%20%3D%20TOOLS%20AND%20labels%20%3D%20community%20and%20status%20%3D%20open)

## Getting Started

1. Create a [MongoDB JIRA account](https://jira.mongodb.org/secure/Signup!default.jspa).
2. Create a [Github account](https://github.com/signup/free).
3. [Fork](https://help.github.com/articles/fork-a-repo/) the repository on Github at
   https://github.com/mongodb/mongo-tools.
4. Sign the [MongoDB Contributor Agreement](https://www.mongodb.com/legal/contributor-agreement).
   This will allow us to review and accept contributions.
5. For more details see http://www.mongodb.org/about/contributors/.
6. Submit a [pull request](https://help.github.com/articles/creating-a-pull-request/) against the
   project for review.

## JIRA Tickets

1. File a JIRA ticket in the [TOOLS project](https://jira.mongodb.org/browse/TOOLS).
2. All commit messages to the MongoDB Tools repository must be prefaced with the relevant JIRA
   ticket number e.g. "TOOLS-XXX add support for xyz".

In filing JIRA tickets for bugs, please clearly describe the issue you are resolving, including the
platforms on which the issue is present and clear steps to reproduce.

For improvements or feature requests, be sure to explain the goal or use case, and the approach your
solution will take.

## Adding or Updating Dependencies

There's some "paperwork" that needs to be done for all dependency changes. To simplify this there
are several command you can run:

```
# Adds the latest version.
go run build.go addDep -pkg=github.com/some/package
# Adds the specified version.
go run build.go addDep -pkg=github.com/some/package@v1.2.3
# Updates to the latest version.
go run build.go updateDep -pkg=github.com/some/package
# Updates to the specified version.
go run build.go updateDep -pkg=github.com/some/package@v1.2.3
# Updates all dependencies to their latest versions.
go run build.go updateAllDeps
```

Note that to run this command you will need to have
[Podman installed](https://podman.io/docs/installation).

This will update our `go.{mod,sum}` files, vendor the dependency, update the SBOM Lite file
(`cyclonedx.sbom.json`), and update the `THIRD-PARTY-NOTICES` file.

Note that you _cannot_ just use `go get` to add or update dependencies, because it doesn't update
all of these other files that need to be updated when dependencies change.

## Testing

You will need a MongoDB server listening on `localhost:33333` to run the integration tests locally.
You can use the [`mlaunch` tool](http://blog.rueckstiess.com/mtools/mlaunch.html) to make this
simple:

```
$> mlaunch init --replicaset --port 33333
```

To run unit and integration tests:

```
go test -v ./...
```

If `TOOLS_TESTING_UNIT` is set to a true value in the shell environment, unit tests will run.

If `TOOLS_TESTING_INTEGRATION` is set to a true value in the shell environment, integration tests
will run.

Integration tests require a `mongod` (running on port 33333) while unit tests do not.

Example of how to run a specific integration test:

```
TOOLS_TESTING_INTEGRATION=true go test -v ./... -run TestImportDocuments
```

To run the quality assurance tests, you need to have the latest stable version of the rebuilt tools,
`mongod`, `mongos`, and `mongo` in your current working directory.

```
cd test/qa-tests
python buildscripts/smoke.py bson export files import oplog restore stat top
```

_Some tests require older binaries that are named accordingly (e.g. `mongod-2.4`, `mongod-2.6`,
etc). You can use
[setup_multiversion_mongodb.py](test/qa-tests/buildscripts/setup_multiversion_mongodb.py) to
download those binaries_

### Writing Tests

In the past, we used
[`github.com/smartystreets/goconvey`](https://pkg.go.dev/github.com/smartystreets/goconvey/convey)
as a test harness. However, we are moving to using to
[`github.com/stretchr/testify`](https://pkg.go.dev/github.com/stretchr/testify) instead. If you are
working with existing tests, it's fine to keep using convey, but **never mix convey and testify in a
single top-level `TestX` func**. If you like, you can also rewrite the test func you're working on
to use `testify`. **All new test funcs should use `testify`.**

#### `require` Versus `assert`

Testify has two primary packages, `assert` and `require`. They provide exactly the same functions.
The only difference is that when a test uses `require` a failure aborts the execution of that test
function.

In general, we prefer to use `require` over `assert`, except in cases where we are sure that a test
failure does not make the following tests invalid. If you're not sure which to use, use `require`.

#### Use the `Assertions` Structs

When using `require` or `assert`, you can either use it via functions or by creating an `Assertions`
struct. The only difference is that the struct holds onto the `*testing.T` struct so you don't have
to pass it to ever assertion call you make:

```go
func TestSomething(t *testing.T) {
    require = require.New(t)
    val := callSomeFunc()
    // We don't pass `t` here:
    require.Equal(42, val, "callSomeFunc returns 42")
}
```

versus:

```go
func TestSomething(t *testing.T) {
    val := callSomeFunc()
    // We do pass `t` here:
    require.Equal(t, 42, val, "callSomeFunc returns 42")
}
```

We use the first style exclusively. The call to `require.New` should always be the first line of
code in any test function, whether that's a top-level `TestX` func or a helper function.

#### Always Check Errors with `require.NoError`

Use the `require.NoError` method to check errors:

```go
err := doSomethingFallible()
require.NoError(err, "doSomethingFallible did not return an error")
```

#### Always Include an Assertion Description

All test assertions should include a description as their final argument. The description should
describe what we expected to happen, not the failure. Here's an example:

```go
require := require.New(t)
val, err := callSomeFunc()
require.NoError(err, "callSomeFunc does not return an error")
require.Equal(42, val, "callSomeFunc returns 42")
```

#### Don't Use the Assertion Methods Ending in "f"

**For example, always use `Equal`, not `Equalf`.** In practice, the "f" variants work in the same
way as their counterparts, so we will just pick the "f"-less version for consistency.

#### Subtests

Use the `t.Run()` method to group tests into subtests. In particular, use this to avoid having very
long `TestX` funcs. For example:

```go
func TestX(t *testing.T) {
    doSomeSetup(t)
    t.Run("variation 1", testVariation1)
    t.Run("variation 2", testVariation2)
    t.Run("variation 3", testVariation3)
}

func testVariation1(t *testing.T) {
    require := require.New(t)
    // Check some assertions
}

func TestY(t *testing.T) {
    db := openDB(t)
    t.Run("variation 1", func (*testing.T) { testVariationWithDb1(t, db) })
    t.Run("variation 2", func (*testing.T) { testVariationWithDb2(t, db) })
    t.Run("variation 3", func (*testing.T) { testVariationWithDb3(t, db) })
}

func testVariationWithDb1(t *testing.T, db *mongo.Database) {
}

```

#### Temp Directories

Many tests need to write data to disk. Whenever possible, use a temp directory for this. You can use
the `testutil.MakeTempDir` function to make a temp directory. If the `TOOLS_TESTING_NO_CLEANUP` env
var is set to a non-empty value then the cleanup func returned by `testutil.MakeTempDir` won't
delete the directory, which is useful when investigating test failures.

#### Example

For an example of all of this, see the `TestRestoreClusteredIndex` func in
`mongorestore/mongorestore_test.go`.

### Static Analysis with `gosec`

We use the `gosec` tool for static analysis of this codebase. You can run this as part of our
linting checks by running the following command:

`go run build.go sa:lint`

If `gosec` reports a vulnerability, you have two options:

1. Fix the issue so that `gosec` stops reporting it.
2. Mark the issue as a false positive using a `#nosec` comment.

If you mark it as a false positive, you _must_ include a justification with the comment:

```
// #nosec G1234 -- the text here explains why we consider this a false positive
```

The justification text will end up in [the SARIF report](https://sarifweb.azurewebsites.net/) we
generate as part of the release process.

**We do not merge PRs which contain unaddressed high- or critical-severity vulnerabilities.**

### Third-Party Dependency Vulnerability Handling

We use [Kondukto](http://kondukto.io/), a third-party SaaS tool, to scan for third-party dependency
vulnerabilities. Kondukto will create Jira tickets in the `VULN` project for any vulnerabilities it
finds. Our Jira instance is set up to then create a linked `TOOLS` ticket.

**We do not merge PRs which contain unaddressed vulnerabilities in third-party dependencies unless
there is no fixed version available. All vulnerabilities found in the `master` branch must be
resolved before a release.**

There are more details about how we handle vulnerabilities in [the release docs](RELEASE.md)

### Software Security Development Lifecycle (SSDLC) Notes

As part of MongoDB's SSDLC initiative, we've made a number of changes to our development practices.
Several of these have already been covered, notably our use of static analysis, the SBOM file, and
third-party vulnerability management.

### Glossary

#### SSDLC: Software Security Development Lifecycle

The practices that we are adopting, including producing an SBOM for all releases, doing various
types of vulnerability scanning, signing releases, and documentation of all these things.

#### SARIF: The Static Analysis Results Interchange Format

This is a file format that static analysis tools can output. See https://sarifweb.azurewebsites.net/
for more information.

#### SBOM: Software Bill of Materials

A machine-readable file containing information about dependencies, including things like the package
name, license, etc. This includes a recursive list of all third-party dependencies.

#### [Kondukto](http://kondukto.io/)

[Kondukto](http://kondukto.io/) is a third-party SaaS tool that MongoDB as a whole uses for managing
SBOMs and third-party vulneerability scanning for our projects. Kondukto is integrated with our Jira
instance so that it can do things like create tickets for vulnerabilities in a project’s
dependencies.

#### Static Analysis

A static analysis tool analyzes code without running it, looking for various issues. For this
particular project, we’re interested in tools that look for security vulnerabilities. For example,
the `gosec` tool attempts to detect when code creates files with insecure permission.

See the "Static Analysis with `gosec`" section above for more details on how to run this tool.

### SBOM Files

We actually have _two_ SBOM files. The first, called the **SBOM Lite** file, lives permanently in
this repo's root as the `cyclonedx.sbom.json` file. This file contains a manifest of all of our
dependencies, including transitive dependencies. It includes information on those package's names,
versions, licenses, and other metadata. However, it does _not_ contain information about
vulnerabilities. It must be updated whenever our dependencies change, and we enforce this via CI.
See the section on "Adding or Updating Dependencies" for more details on how to do this.

Vulnerability information lives in our **Augmented SBOM** files. These files live in the `ssdlc`
directory, and we create a new one for each release. These files act as a record of our
dependencies, including known vulnerabilities, for each release. The releases include the tag name
of the release, for example `ssdlc/100.9.5.bom.json`. This file must be created for each release,
and we enforce this via CI.

#### Generating the Augmented SBOM File

Generating this file can only be done by MongoDB employees, as it requires access to
[Kondukto](http://kondukto.io/). See our [release documentation](./RELEASE.md) for more details.

### Papertrail Integration

All releases are recorded using a MongoDB-internal application called Papertrail. This records
various pieces of information about releases, including the date and time of the release, who
triggered the release (by pushing to Evergreen), and a checksum of each release file.

This is done automatically as part of the release.

### Release Artifact Signing

All releases are signed automatically as part of the release process.
