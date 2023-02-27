Contributing to the MongoDB Tools Project
===================================

Pull requests are always welcome, and the MongoDB engineering team appreciates any help the community can give to make the MongoDB tools better.

For any particular improvement you want to make, you can begin a discussion on the
[MongoDB Developers Forum](https://groups.google.com/forum/?fromgroups#!forum/mongodb-dev).  This is the best place to discuss your proposed improvement (and its
implementation) with the core development team.

If you're interested in contributing, we have a list of some suggested tickets that are easy enough to get started on [here](https://jira.mongodb.org/issues/?jql=project%20%3D%20TOOLS%20AND%20labels%20%3D%20community%20and%20status%20%3D%20open)

Getting Started
---------------

1. Create a [MongoDB JIRA account](https://jira.mongodb.org/secure/Signup!default.jspa).
2. Create a [Github account](https://github.com/signup/free).
3. [Fork](https://help.github.com/articles/fork-a-repo/) the repository on Github at https://github.com/mongodb/mongo-tools.
4. Sign the [MongoDB Contributor Agreement](https://www.mongodb.com/legal/contributor-agreement). This will allow us to review and accept contributions.
5. For more details see http://www.mongodb.org/about/contributors/.
6. Submit a [pull request](https://help.github.com/articles/creating-a-pull-request/) against the project for review.

JIRA Tickets
------------

1. File a JIRA ticket in the [TOOLS project](https://jira.mongodb.org/browse/TOOLS).
2. All commit messages to the MongoDB Tools repository must be prefaced with the relevant JIRA ticket number e.g. "TOOLS-XXX add support for xyz".

In filing JIRA tickets for bugs, please clearly describe the issue you are resolving, including the platforms on which the issue is present and clear steps to reproduce.

For improvements or feature requests, be sure to explain the goal or use case, and the approach
your solution will take.

Style Guide
-----------

All commits to the MongoDB Tools repository must pass golint:

```go run vendor/github.com/3rf/mongo-lint/golint/golint.go mongo* bson* common/*```

_We use a modified version of [golint](https://github.com/golang/lint)_

Testing
-------

You will need a MongoDB server listening on `localhost:33333` to run the integration tests locally. You can use the [`mlaunch` tool](http://blog.rueckstiess.com/mtools/mlaunch.html) to make this simple:

```
$> mlaunch init --replicaset --port 33333
```

To run unit and integration tests:

```
go test -v ./...
```
If TOOLS_TESTING_UNIT is set to a true value in the environment, unit tests will run.
If TOOLS_TESTING_INTEGRATION is set to a true value in the environment, integration tests will run.

Integration tests require a `mongod` (running on port 33333) while unit tests do not.

To run the quality assurance tests, you need to have the latest stable version of the rebuilt tools, `mongod`, `mongos`, and `mongo` in your current working directory.

```
cd test/qa-tests
python buildscripts/smoke.py bson export files import oplog restore stat top
```
_Some tests require older binaries that are named accordingly (e.g. `mongod-2.4`, `mongod-2.6`, etc). You can use [setup_multiversion_mongodb.py](test/qa-tests/buildscripts/setup_multiversion_mongodb.py) to download those binaries_

### Writing Tests

In the past, we used [`github.com/smartystreets/goconvey`](https://pkg.go.dev/github.com/smartystreets/goconvey/convey) as a test harness. However, we are moving to using to [`github.com/stretchr/testify`](https://pkg.go.dev/github.com/stretchr/testify) instead. If you are working with existing tests, it's fine to keep using convey, but **never mix convey and testify in a single top-level `TestX` func**. If you like, you can also rewrite the test func you're working on to use `testify`. **All new test funcs should use `testify`.**

#### `require` Versus `assert`

Testify has two primary packages, `assert` and `require`. They provide exactly the same functions. The only difference is that when a test uses `require` a failure aborts the execution of that test function.

In general, we prefer to use `require` over `assert`, except in cases where we are sure that a test failure does not make the following tests invalid. If you're not sure which to use, use `require`.

#### Use the `Assertions` Structs

When using `require` or `assert`, you can either use it via functions or by creating an `Assertions` struct. The only difference is that the struct holds onto the `*testing.T` struct so you don't have to pass it to ever assertion call you make:


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

We use the first style exclusively. The call to `require.New` should always be the first line of code in any test function, whether that's a top-level `TestX` func or a helper function.

#### Always Check Errors with `require.NoError`

Use the `require.NoError` method to check errors:

```go
err := doSomethingFallible()
require.NoError(err, "doSomethingFallible did not return an error")
```

#### Always Include an Assertion Description

All test assertions should include a description as their final argument. The description should describe what we expected to happen, not the failure. Here's an example:

```go
require := require.New(t)
val, err := callSomeFunc()
require.NoError(err, "callSomeFunc does not return an error")
require.Equal(42, val, "callSomeFunc returns 42")
```

#### Don't Use the Assertion Methods Ending in "f"

**For example, always use `Equal`, not `Equalf`.** In practice, the "f" variants work in the same way as their counterparts, so we will just pick the "f"-less version for consistency.

#### Subtests

Use the `t.Run()` method to group tests into subtests. In particular, use this to avoid having very long `TestX` funcs. For example:

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

Many tests need to write data to disk. Whenever possible, use a temp directory for this. You can use the `testutil.MakeTempDir` function to make a temp directory. If the `TOOLS_TESTING_NO_CLEANUP` env var is set to a non-empty value then the cleanup func returned by `testutil.MakeTempDir` won't delete the directory, which is useful when investigating test failures.

#### Example

For an example of all of this, see the `TestRestoreClusteredIndex` func in `mongorestore/mongorestore_test.go`.
