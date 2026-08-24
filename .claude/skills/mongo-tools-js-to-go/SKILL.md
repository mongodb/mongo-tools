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

- **Callers before callees**: test functions before helpers, helpers before the helpers they call. A helper used by one test goes immediately below that test, not at the bottom of the file. A helper that creates or sets up collections for several tests goes in `suite_test.go` with the existing create-collection helpers, not in the test file.
- **No comments that describe what code is doing** — use named functions, subtests, and descriptive variable names instead. Comments explaining *why* are fine.
- Use `any` not `interface{}`
- Use `bson.D` — not `bson.M` — for documents, filters, and commands, matching the rest of
  the codebase. Build `bson.D`/`bson.E` literals unkeyed.
- Use `for i := range n`, not `for i := 0; i < n; i++`.
- Use a set for membership checks, rather than scanning a slice. Sets come from
  `mapset "github.com/deckarep/golang-set/v2"` (aliased on import, as the rest of the codebase
  does) — not a `map[string]struct{}`:

    ```go
    var systemDBs = mapset.NewSet("admin", "local", "config")
    if systemDBs.Contains(name) { ... }
    ```
- Always include assertion messages: `assert.Equal(t, want, got, "description of what is being tested")`
- Reset `map[string]any{}` before each `Decode` call — stale keys from previous decodes persist otherwise
- Table-driven tests: define a `type fooCase struct` and loop over `[]fooCase`
- For error cases, don't use `require.Error` Use one of the following:
    - `require.ErrorIs(t, err, something)`
    - `require.ErrorAs(t, err, &var)`
    - `require.ErrorContains(t, err, "substring")`

## Naming and Scope

Reviewers raise these on nearly every conversion PR. Get them right the first time.

- **Declare a constant where it is used.** A `const` used inside exactly one function belongs
  inside that function. Package-level constants are for values several functions share.
- **Don't name a constant for a value that is arbitrary.** If the test just needs "some
  documents", write the number inline or use a plain variable; `const fooDocCount = 7` invites
  a reviewer to hunt for the significance of 7 and find none. Constants are worth naming when
  the value is load-bearing (a size limit, a timeout the tool cares about).
- **An assertion message describes that one assertion**, not the test around it. If the
  assertion checks that a spec has a name, say that — not that indexes round-trip.
- **A helper's name has to match what it does.** `assertFooRestored` that only counts
  documents is misnamed, and a new `restoreFromArgs` sitting next to an existing
  `getRestoreWithArgs` that does something different will confuse everyone. Check for an
  existing helper with a similar name before adding one.
- **One word, one meaning per file.** If "fixture" means a struct with attached state in one
  place, don't use it for a function that only inserts documents in another.
- **Arbitrary payload data should be boring.** Filler strings like `"drop"` or `"shard"` read
  as meaningful and send reviewers looking for behavior that isn't there. Use obviously inert
  values.
- **Prefer a top-level test to a subtest that gains nothing.** Use `s.Run`/`t.Run` when the
  cases share setup or form a table, not to group otherwise independent tests.

## Don't Talk About the JS Test in the Go Code

Reviewers have asked for this repeatedly. A comment saying "this converts `foo.js`", "the JS
test's intent was...", or otherwise narrating the provenance of a test is not useful to anyone
reading the Go code later. Delete them.

Provenance belongs in the **commit message**, which is also the PR description: name the JS
files the commit converts and deletes there.

The one comment worth keeping in the code is a **coverage note**: when the Go test deliberately
does *not* cover something the JS test did, say so where the test is, because that is a fact
about the current test rather than a fact about history.

## Fidelity to the Original

The reviewer reads the deleted JS alongside the new Go, so any divergence gets noticed.

- **Don't add behavior the JS test didn't have.** An extra assertion the original never made
  can fail for reasons that have nothing to do with what is being tested. If you think a new
  assertion is worth having, be able to say why it should be able to fail.
- **Don't quietly drop behavior either.** If the JS test runs the tool ten times, passes a `v`
  field, or dumps from one cluster and restores to another, either reproduce it or call it out
  explicitly in the PR description as coverage not carried over. Both of those have been caught
  in review.
- **Match the target package's style, not the JS file's structure.** A test that runs dump and
  restore goes in `integration/dumprestore` and looks like the tests already there, even if the
  JS version lived somewhere else.
- **Flag behavior we no longer support.** Some JS tests cover things like restoring from a v2.6
  dump, which the tools no longer do. Convert it if that is what was asked for, but tell the
  user, so they can decide whether to delete the test and the code behind it instead.

## Keep PRs Reviewable

A reviewer has told us directly that these PRs are hard to review, mostly because of size — one
was nearly 2,000 lines. Reviewing a conversion means reading the deleted JS carefully *and* the
new Go carefully.

- Target 200-400 lines per PR, as `AGENTS.md` says. Split by test file or by theme rather than
  converting a whole directory at once.
- Write plain, unremarkable Go. Code that is "correct but nothing a human would write" costs
  review time even when it works.
- Avoid ornate wording in comments and test names. Plain description beats clever phrasing.

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
    * Anything the JS test covered that the Go test does not
6. Run a sub-agent that does not share the current context. Ask that agent to provide a review of the PR. This review should ensure the following things:
    * The new Go test covers the same things that the JS test does, and adds nothing the JS test did not do.
    * The new Go test follows all of the guidelines for Go tests in this skill, particularly the Naming and Scope rules and the ban on comments referring to the JS test.
    * The new Go test is generally similar to other Go tests that use `testify`.
    * Every assertion can actually fail. A test that would pass against a broken tool is worse than no test.
7. Share the review with the user and ask if they'd like you to make further changes based on the review
8. Prompt the user before deleting the JS file
9. Delete the JS file
10. Check the size of the diff. If it is much over 400 lines, propose splitting it before asking for review.

## Committing

- The commit message should start with a TOOLS ticket number, like `TOOLS-1234`. Ask the user which
  ticket to use if you don't know which one is being used for this work.
- Auto-fix formatting before committing: `precious tidy -g`
- Lint check: `precious lint -g`

## Linting Notes

- `bson.E struct literal uses unkeyed fields` — suppressed by `.golangci.yml`, ignore it
- `precious tidy -g` fixes indentation and line-length issues automatically
