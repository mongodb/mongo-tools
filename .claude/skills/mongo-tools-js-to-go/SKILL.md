---
name: mongo-tools-js-to-go
description: Use when converting JS/resmoke integration tests in mongo-tools to Go testify tests
---

# mongo-tools JS-to-Go Test Conversion

## Integration Test Boilerplate

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

## Code Conventions

- **Callers before callees**: test functions before helpers, helpers before the helpers they call
- **No comments that describe what code is doing** — use named functions, subtests, and descriptive variable names instead. Comments explaining *why* are fine.
- Use `any` not `interface{}`
- Always include assertion messages: `assert.Equal(t, want, got, "description of what is being tested")`
- Reset `map[string]any{}` before each `Decode` call — stale keys from previous decodes persist otherwise
- Table-driven tests: define a `type fooCase struct` and loop over `[]fooCase`
- For error cases, don't use `require.Error` Use one of the following:
    - `require.ErrorIs(t, err, something)
    - `require.ErrorAs(t, err, &var)
    - `require.ErrorContains(t, err, "substring")`

## Round-Trip Tests (export+import or dump+restore)

Round-trip tests belong in **mongoimport** (not mongoexport) and **mongorestore** (not mongodump).

**Critical:** drop the collection between export/dump and import/restore. Without this, the restore can't be verified.

```go
_, err = me.Export(tmpFile)
require.NoError(t, err)
require.NoError(t, tmpFile.Close())

require.NoError(t, coll.Drop(t.Context())) // ← required

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

```bash
TOOLS_TESTING_INTEGRATION=true \
TOOLS_TESTING_AUTH=1 \
TOOLS_TESTING_AUTH_USERNAME=username \
TOOLS_TESTING_AUTH_PASSWORD=password \
go test ./mongoimport/... -v -run TestFoo -count=1
```

Always run the test locally before committing.

## Conversion Process

1. Read the JS test, note what testtype it requires
2. Write the Go test; prefer round-trip tests for export+import scenarios
    * Round-trip tests should be moved to the right subdir under `integration`.
3. Run locally, confirm it passes
4. Prompt the user before deleting the JS file
5. Delete the JS file

## Committing

- The commit message should start with a TOOLS ticket number, like `TOOLS-1234`. Ask the user which
  ticket to use if you don't know which one is being used for this work.
- Auto-fix formatting before committing: `precious tidy -g`
- Lint check: `precious lint -g`

## Linting Notes

- `bson.E struct literal uses unkeyed fields` — suppressed by `.golangci.yml`, ignore it
- `precious tidy -g` fixes indentation and line-length issues automatically
