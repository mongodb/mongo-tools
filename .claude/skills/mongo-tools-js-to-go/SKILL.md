---
name: mongo-tools-js-to-go
description: Use when converting JS/resmoke integration tests in mongo-tools to Go testify tests
---

# mongo-tools JS-to-Go Test Conversion

## Integration Test Boilerplate

For tests within an individual tool's package (e.g. `mongoimport/`, `mongodump/`):

```go
func TestFoo(t *testing.T) {
    testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

    const (
        dbName   = "mongofoo_test_db"
        collName = "coll"
    )

    sessionProvider, _, err := testutil.GetBareSessionProvider()
    require.NoError(t, err)
    client, err := sessionProvider.GetSession()
    require.NoError(t, err)
    t.Cleanup(func() {
        _ = client.Database(dbName).Drop(context.Background())
    })

    coll := client.Database(dbName).Collection(collName)
    ns := &options.Namespace{DB: dbName, Collection: collName}
    // ...
}
```

Test type constants: `IntegrationTestType`, `ReplSetTestType`, `ShardedIntegrationTestType`, `SSLTestType`, `AuthTestType`.

## E2E Tests (Integration Suite)

Tests that exercise the full tool pipeline (dump+restore or export+import) belong in `integration/dumprestore` or `integration/exportimport` and use **testify suites**.

Add tests as methods on the existing suite type for the relevant package. The suite entry point looks like:

```go
type DumpRestoreSuite struct {
    integrationSuite.IntegrationSuite
}

func TestDumpRestore(t *testing.T) {
    testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)
    suite.Run(t, new(DumpRestoreSuite))
}
```

Suite test methods:

```go
func (s *DumpRestoreSuite) TestFoo() {
    ctx := s.Context()
    client := s.Client()
    dbName := s.DBName()
    // use s.Require() / s.Assert() instead of require.New(t)
}
```

Key suite methods:

| Method | Purpose |
|---|---|
| `s.Context()` | Test-scoped context |
| `s.Client()` | New MongoDB client (caller responsible for Disconnect) |
| `s.DBName(prefix...)` | DB name derived from test name, truncated to 63 chars |
| `s.Require()` | testify require bound to current (sub)test |
| `s.Assert()` | testify assert bound to current (sub)test |
| `s.T()` | Current `*testing.T` |
| `s.Run(name, func())` | Subtest (updates `s.T()` for the duration) |

**No manual DB cleanup needed** — `BeforeTest` in `IntegrationSuite` drops all non-system databases before each test method. Do not register `t.Cleanup` DB drops in suite tests.

## Code Conventions

- **Callers before callees**: test functions before helpers, helpers before the helpers they call
- **No comments that describe what code is doing** — use named functions, subtests, and descriptive variable names instead. Comments explaining *why* are fine.
- Use `any` not `interface{}`
- Always include assertion messages: `assert.Equal(t, want, got, "description of what is being tested")`
- Reset `map[string]any{}` before each `Decode` call — stale keys from previous decodes persist otherwise
- Table-driven tests: define a `type fooCase struct` and loop over `[]fooCase`
- For error cases, don't use `require.Error` Use one of the following:
    - `require.ErrorIs(t, err, something)`
    - `require.ErrorAs(t, err, &var)`
    - `require.ErrorContains(t, err, "substring")`

## Round-Trip Tests (export+import or dump+restore)

Round-trip tests belong in `integration/exportimport` or `integration/dumprestore` (suite methods), not in the individual tool packages.

**Critical:** drop the collection between export/dump and import/restore. Without this, the restore can't be verified.

```go
_, err = me.Export(tmpFile)
s.Require().NoError(err)
s.Require().NoError(tmpFile.Close())

s.Require().NoError(coll.Drop(s.Context())) // ← required

// now import and verify
```

## JSON Test File Generation

Use Go data structures + `json.Marshal` (not hardcoded strings):

```go
upsertFile := writeJSONLinesFile(t, dir, "data.json", []map[string]any{
    {"_id": "one", "a": 1234, "b": "foo"},
    {"_id": "two", "a": "xxx", "b": "yyy"},
})
```

For BSON-type-preserving output (e.g. subdocument _ids), use `bson.MarshalExtJSON(doc, relaxed, escapeHTML)`.

## Key Helpers

| Helper | Purpose |
|---|---|
| `testutil.GetBareSessionProvider()` | Get a live MongoDB client |
| `testutil.GetToolOptions()` | Get tool options (connects to test mongod) |
| `testutil.GetBareArgs()` | CLI args (`--host`, `--port`, auth) for `exec.Command` |
| `runImportOpts(t, ns, file, IngestOptions{})` | Import, returns errors from `New()` too (use for option-validation tests) |
| `importWithIngestOpts(t, ns, file, IngestOptions{})` | Import, fails test if `New()` errors |
| `testutil.AssertBrokenPipeHandled(t, cmd)` | Verify a process handles SIGPIPE as a write error |

## Running Tests Locally

Tool-package test:

```bash
TOOLS_TESTING_INTEGRATION=true \
go test ./mongoimport/... -v -run TestFoo -count=1
```

Suite (e2e) test:

```bash
TOOLS_TESTING_INTEGRATION=true \
go test ./integration/dumprestore/... -v -run TestDumpRestore/TestFoo -count=1
```

Add `TOOLS_TESTING_AUTH=1 TOOLS_TESTING_AUTH_USERNAME=... TOOLS_TESTING_AUTH_PASSWORD=...` when testing against an auth-enabled mongod.

Always run the test locally before committing.

## Conversion Process

1. Read the JS test, note what testtype it requires
2. Decide which form to use:
    * **Full-pipeline e2e** (dump+restore, export+import): add a method to the existing suite in `integration/dumprestore` or `integration/exportimport`
    * **Tool-unit / single-tool**: plain `func TestFoo(t *testing.T)` inside the tool's own package
3. Write the Go test; prefer round-trip tests for export+import scenarios
4. Run locally, confirm it passes
5. Provide the user with a detailed explanation of the work. This should include:
    * What the JS test was testing
    * How the Go test tests the same thing
6. Run a sub-agent that does not share the current context. Ask that agent to provide a review of the PR. This review should ensure the following things:
    * The new Go test covers the same things that the JS test does.
    * The new Go test follows all of the guidelines for Go tests in this skill.
    * The new Go test is generally similar to other Go tests that use `testify`.
7. Share the review with the user and ask if they'd like you to make further changes based on the review
8. Prompt the user before deleting the JS file
9. Delete the JS file

## Committing

- The commit message should start with a TOOLS ticket number, like `TOOLS-1234`. Ask the user which
  ticket to use if you don't know which one is being used for this work.
- Auto-fix formatting before committing: `precious tidy -g`
- Lint check: `precious lint -g`

## Linting Notes

- `bson.E struct literal uses unkeyed fields` — suppressed by `.golangci.yml`, ignore it
- `precious tidy -g` fixes indentation and line-length issues automatically
