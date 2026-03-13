# JS-to-Go Test Migration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert ~160 JavaScript integration tests (run via resmoke) to Go integration tests using testify, then delete the JS test infrastructure.

**Architecture:** Each JS test becomes a Go integration test in the existing tool package (e.g., `jstests/dump/*.js` → `mongodump/`). Tests call tool library code directly rather than spawning processes. New test functions follow the existing repo pattern: `testtype.SkipUnlessTestType`, `testutil` helpers, `testify/require` and `testify/assert` for assertions — no GoConvey.

**Tech Stack:** Go, `github.com/stretchr/testify`, `go.mongodb.org/mongo-driver/v2`, `github.com/mongodb/mongo-tools/common/testtype`, `github.com/mongodb/mongo-tools/common/testutil`

---

## Reference: Go Integration Test Pattern

Every test in this migration must follow this pattern. Read an existing large test file (e.g., `mongodump/mongodump_test.go`) to see it in context.

```go
func TestFooBar(t *testing.T) {
    testtype.SkipUnlessTestType(t, testtype.IntegrationTestType)

    // Get a live client. Connects to localhost:33333 or $TOOLS_TESTING_MONGOD.
    client, err := testutil.GetBareSession()
    require.NoError(t, err)
    defer client.Disconnect(context.Background())

    db := client.Database("mongofoo_test_db")
    defer db.Drop(context.Background()) // clean up after the test

    t.Run("does the thing", func(t *testing.T) {
        // Insert test data
        _, err := db.Collection("coll").InsertOne(context.Background(), bson.D{{"k", "v"}})
        require.NoError(t, err)

        // Invoke the tool via its library API (not exec.Command):
        opts, err := testutil.GetToolOptions()
        require.NoError(t, err)
        opts.Namespace = &options.Namespace{DB: "mongofoo_test_db", Collection: "coll"}

        tool := &SomeTool{ToolOptions: opts, ...}
        err = tool.DoThing()
        require.NoError(t, err)

        // Assert outcomes
        var result bson.D
        err = db.Collection("coll").FindOne(context.Background(), bson.D{}).Decode(&result)
        require.NoError(t, err)
        assert.Equal(t, "v", result.Map()["k"])
    })
}
```

**Running integration tests:**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongodump/... -v -run TestFooBar
```

**Running all integration tests for a package:**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongodump/... -v -count=1
```

---

## Reference: JS Test Infrastructure to Delete (End State)

After all batches are complete:

- `test/qa-tests/` — entire directory
- `test/legacy42/` — entire directory
- `scripts/run_qa.sh`
- `scripts/run_native_cert_ssl.sh`
- Remove `qa-tests-*`, `legacy-jstests-*`, `native-cert-ssl-*` tasks from `common.yml`
- Update `build.go` to remove resmoke-related build targets (if any)

---

## File Structure

Each batch extends or creates test files in these locations:

| JS location | Go target | Notes |
|---|---|---|
| `jstests/bson/*.js` | `bsondump/bsondump_test.go` | Extend existing file |
| `jstests/dump/*.js` | `mongodump/mongodump_qa_test.go` | New file; existing test file is 2537 lines |
| `jstests/export/*.js` | `mongoexport/mongoexport_test.go` | Extend existing file (331 lines) |
| `jstests/import/*.js` | `mongoimport/mongoimport_test.go` | Extend existing file |
| `jstests/restore/*.js` | `mongorestore/mongorestore_qa_test.go` | New file; existing test file is 3879 lines |
| `jstests/files/*.js` | `mongofiles/mongofiles_test.go` | Extend existing file |
| `jstests/stat/*.js` | `mongostat/mongostat_test.go` | Extend existing file |
| `jstests/top/*.js` | `mongotop/mongotop_test.go` | New file; currently only `options_test.go` exists |
| `jstests/ssl/*.js` | `mongoexport/mongoexport_test.go` | Under `testtype.SSLTestType` |
| `jstests/tls/*.js` | `mongoexport/mongoexport_test.go` | Under `testtype.SSLTestType` |
| `test/legacy42/jstests/tool/*.js` | Various (see Batch 10) | Audit first; most are duplicates |

---

## Chunk 1: Audit + bsondump

### Task 0: Add ShardedTestType infrastructure

Several JS tests require a sharded cluster (mongos). Rather than skipping them, we introduce a new `ShardedTestType` parallel to the existing `ReplSetTestType`.

**Files to modify:**
- `common/testtype/types.go` — add the constant
- `common.yml` — add a sharded build variant that sets `TOOLS_TESTING_SHARDED=1` and starts a mongos

- [ ] **Step 1: Add the constant to `common/testtype/types.go`**

Add after the `ReplSetTestType` constant:

```go
// Testing the tools against a sharded cluster (mongos) topology.
ShardedTestType = "TOOLS_TESTING_SHARDED_INTEGRATION"
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./common/testtype/...
```

Expected: no errors.

- [ ] **Step 3: Add a sharded build variant to `common.yml`**

Find the existing `integration-X.Y-cluster` task family (replica set topology tasks) and add a new family of tasks for sharded topologies. The pattern to follow is the same as the existing `integration-X.Y-auth` tasks: a new expansion variable that is set in the build variant and consumed by the test runner.

The key expansion to add in the new build variant:

```yaml
expansions:
  test_flags: "TOOLS_TESTING_SHARDED=1"
  topology: sharded
```

And the `start-mongod` function (or equivalent) for that variant must start a sharded cluster instead of a standalone. Follow the pattern already used for the `cluster` variants — those already start a replica set via `ReplSetTest`; the sharded variant should start a `ShardingTest` instead.

After editing `common.yml`, regenerate and validate:

```bash
go run build.go sa:evgvalidate
```

Expected: validation passes.

- [ ] **Step 4: Commit**

```bash
git add common/testtype/types.go common.yml
git commit -m "testtype: add ShardedTestType for sharded cluster integration tests"
```

---

### Task 1: Audit all JS tests against existing Go tests

Before converting anything, map each JS test file to: **SKIP** (already covered in Go), **EXTEND** (partial coverage, add cases), or **NEW** (no coverage).

**Files to audit:**
- All files in `test/qa-tests/jstests/bson/`, `dump/`, `export/`, `import/`, `restore/`, `files/`, `stat/`, `top/`, `ssl/`, `tls/`, `txn/`
- All files in `test/legacy42/jstests/tool/`

**Files to read for comparison:**
- `bsondump/bsondump_test.go`
- `mongodump/mongodump_test.go` (2537 lines)
- `mongorestore/mongorestore_test.go` (3879 lines), `mongorestore/oplog_test.go`, `mongorestore/dumprestore_auth_test.go`
- `mongoexport/mongoexport_test.go`, `mongoexport/csv_test.go`, `mongoexport/json_test.go`
- `mongoimport/mongoimport_test.go`, `mongoimport/csv_test.go`, `mongoimport/json_test.go`, `mongoimport/tsv_test.go`
- `mongofiles/mongofiles_test.go`
- `mongostat/mongostat_test.go`

- [ ] **Step 1: Read every JS test file** for each tool area. For each file, check the corresponding Go test file for equivalent coverage.

- [ ] **Step 2: Record the triage result** by adding a comment at the top of each JS file:

```js
// MIGRATION: SKIP — covered by TestMongoDumpBSON in mongodump/mongodump_test.go
// MIGRATION: EXTEND — add sub-case to TestMongoDumpViews
// MIGRATION: NEW — no Go coverage exists
```

- [ ] **Step 3: Commit the triage annotations**

```bash
git add test/
git commit -m "migration: annotate JS tests with Go coverage triage"
```

---

### Task 2: Convert bsondump JS tests

**JS files:** `test/qa-tests/jstests/bson/` (7 files)
**Go target:** `bsondump/bsondump_test.go`

First, read `bsondump/bsondump_test.go` in full to understand existing coverage.

The bsondump JS tests primarily test: correct output format for all BSON types (debug and JSON modes), error handling for malformed input, option validation, broken pipe, deep nesting, and output file writing. The tool is invoked as: `bsondump [options] <file>` — call the library: `bsondump.New(opts)` / `bsondump.Dump(writer)`.

- [ ] **Step 1: Convert `all_types.js`** — Add `TestBSONDumpAllTypesDebug` if not covered. Insert a document with every BSON type into a collection, dump it to a temp file, run bsondump on it, verify output contains the correct debug-format type labels.

- [ ] **Step 2: Convert `all_types_json.js`** — Add `TestBSONDumpAllTypesJSON` if not covered. Same setup, verify JSON output mode produces correct Extended JSON.

- [ ] **Step 3: Convert `bad_files.js`** — Add `TestBSONDumpBadInput`. Write temp files containing: random bytes, truncated BSON, bad cstring, unsupported type byte. Verify bsondump returns an error or continues with `--objcheck` as appropriate.

- [ ] **Step 4: Convert `deep_nested.js`** — Add `TestBSONDumpDeepNested`. Create a deeply nested document (100+ levels), dump it, bsondump it, verify no stack overflow or error.

- [ ] **Step 5: Convert `output_file.js`** — Add `TestBSONDumpOutputFile`. Run bsondump with `--out=<tmpfile>`, verify the file is created and contains expected output.

- [ ] **Step 6: Convert `bsondump_options.js`** — Add cases to `TestBSONDumpOptions` or a new `TestBSONDumpOptionValidation`. Verify invalid flag combinations return errors; `--type`, `--help`, `--version` flags behave as expected.

- [ ] **Step 7: Skip `bsondump_broken_pipe.js`** — Broken pipe behavior is OS-signal-level and untestable in Go unit/integration tests without significant complexity. Add a `// MIGRATION: SKIP — broken pipe is OS-level, not testable in Go` comment and leave it out.

- [ ] **Step 8: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./bsondump/... -v -run "TestBSONDump" -count=1
```

Expected: all new tests PASS.

- [ ] **Step 9: Delete the converted JS files**

```bash
rm test/qa-tests/jstests/bson/all_types.js
rm test/qa-tests/jstests/bson/all_types_json.js
rm test/qa-tests/jstests/bson/bad_files.js
rm test/qa-tests/jstests/bson/deep_nested.js
rm test/qa-tests/jstests/bson/output_file.js
rm test/qa-tests/jstests/bson/bsondump_options.js
# Leave bsondump_broken_pipe.js with a SKIP comment, or delete it too
rm test/qa-tests/jstests/bson/bsondump_broken_pipe.js
```

- [ ] **Step 10: Commit**

```bash
git add bsondump/bsondump_test.go test/qa-tests/jstests/bson/
git commit -m "migration: convert bsondump JS tests to Go"
```

---

## Chunk 2: mongoexport + mongoimport

### Task 3: Convert mongoexport JS tests

**JS files:** `test/qa-tests/jstests/export/` (~20 files)
**Go target:** `mongoexport/mongoexport_test.go`

Read the existing `mongoexport_test.go` (331 lines — relatively short), `csv_test.go`, and `json_test.go` before starting.

The JS tests cover: basic data roundtrip, data types, broken pipe, views, field selection (JSON and CSV), force table scan, JSON array output, `--limit`, namespace validation, nested CSV fields, pretty output, `--query`, `--slaveOk`, sort/skip, stdout output, and type case handling.

The mongoexport library API: create `mongoexport.MongoExport{Options: opts}`, then call `export.ExpManager.Open()` and `export.Export(writer)`.

- [ ] **Step 1: For each JS file, check triage annotation** from Task 1. Only convert NEW or EXTEND files.

- [ ] **Step 2: Convert `basic_data.js`** — `TestExportImportBasicRoundtrip`: insert documents, export to buffer, import back, verify document count and values match.

- [ ] **Step 3: Convert `data_types.js`** — `TestExportDataTypes`: insert documents with ObjectId, Date, NumberLong, NumberInt, Decimal128, BinData, Regex, verify export output contains correct Extended JSON representations.

- [ ] **Step 4: Convert `field_file.js` and `fields_json.js` and `fields_csv.js`** — `TestExportFieldSelection`: verify `--fields` limits output to named fields in both JSON and CSV modes.

- [ ] **Step 5: Convert `nested_fields_csv.js`** — `TestExportNestedFieldsCSV`: export with dotted field paths in `--fields`, verify CSV flattening.

- [ ] **Step 6: Convert `json_array.js`** — `TestExportJSONArray`: verify `--jsonArray` flag wraps output in `[...]`.

- [ ] **Step 7: Convert `query.js`** — `TestExportQuery`: insert 10 docs, export with `--query '{"x": {"$gt": 5}}'`, verify only matching docs appear.

- [ ] **Step 8: Convert `sort_and_skip.js`** — `TestExportSortAndSkip`: insert ordered docs, export with `--sort` and `--skip`, verify ordering.

- [ ] **Step 9: Convert `limit.js`** — `TestExportLimit`: insert 20 docs, export with `--limit 5`, verify exactly 5 docs in output.

- [ ] **Step 10: Convert `pretty.js`** — `TestExportPretty`: verify `--pretty` flag produces indented JSON output.

- [ ] **Step 11: Convert `no_data.js`** — `TestExportNoData`: export an empty collection, verify empty output (not an error).

- [ ] **Step 12: Convert `export_views.js`** — `TestExportViews`: create a view, export it, verify exported data matches the view's pipeline output.

- [ ] **Step 13: Convert `namespace_validation.js`** — `TestExportNamespaceValidation`: verify that invalid namespace combinations (no `--db`, `--collection` without `--db`, etc.) return appropriate errors.

- [ ] **Step 14: Convert remaining files** (`stdout.js`, `type_case.js`, `force_table_scan.js`, `slave_ok.js`):
  - `TestExportStdout`: verify export to stdout (write to `os.Stdout` or a pipe).
  - `TestExportTypeCase`: verify type name case insensitivity.
  - `TestExportForceTableScan`: verify `--forceTableScan` flag doesn't break basic export.
  - Skip `slave_ok.js` — `--slaveOk` is deprecated/removed; add a `// MIGRATION: SKIP` comment.
  - Skip `export_broken_pipe.js` — same rationale as bsondump broken pipe.

- [ ] **Step 15: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongoexport/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 16: Delete the JS files, commit**

```bash
rm -r test/qa-tests/jstests/export/
git add mongoexport/mongoexport_test.go mongoexport/csv_test.go mongoexport/json_test.go test/
git commit -m "migration: convert mongoexport JS tests to Go"
```

---

### Task 4: Convert mongoimport JS tests

**JS files:** `test/qa-tests/jstests/import/` (~19 files)
**Go target:** `mongoimport/mongoimport_test.go`

Read `mongoimport_test.go` (1525 lines), `csv_test.go`, `json_test.go`, `tsv_test.go`, and `typed_fields_test.go` before starting.

The JS tests cover: write concerns (standalone and mongos), `--drop` flag, `--fields`, document validation, type preservation, `--mode` (upsert/insert/merge/delete), upsert with subdocument IDs, parse grace, replica set behavior, `--stopOnError`, error codes for no-primary, boolean types, Decimal128, typed fields.

The mongoimport API: `mongoimport.MongoImport{ToolOptions: opts, IngestOptions: ingestOpts, InputOptions: inputOpts}`, then `mi.ImportDocuments()`.

- [ ] **Step 1: Triage** — check which JS tests have Go equivalents in `mongoimport_test.go`.

- [ ] **Step 2: Convert `drop.js`** — `TestImportDrop`: import docs, import again with `--drop`, verify collection was replaced not appended.

- [ ] **Step 3: Convert `mode.js`** — `TestImportModes`: test each mode (insert, upsert, merge, delete) by importing data over pre-existing documents and verifying the resulting state.

- [ ] **Step 4: Convert `mode_upsert_id_subdoc.js`** — `TestImportUpsertSubdocumentID`: import documents whose `_id` is a subdocument, verify upsert uses the full subdoc as the key.

- [ ] **Step 5: Convert `import_document_validation.js`** — `TestImportDocumentValidation`: create a collection with a `$jsonSchema` validator, import documents that violate it, verify appropriate error or bypass behavior.

- [ ] **Step 6: Convert `stoponerror.js`** — `TestImportStopOnError`: import a file with one bad document; with `--stopOnError`, verify import halts; without it, verify import continues.

- [ ] **Step 7: Convert `boolean_type.js`** — `TestImportBooleanType`: verify boolean values in CSV/JSON import are preserved as BSON booleans.

- [ ] **Step 8: Convert `decimal128.js`** — `TestImportDecimal128`: verify Decimal128 values in Extended JSON are correctly imported as BSON Decimal128.

- [ ] **Step 9: Convert `fields.js`** — `TestImportFields`: verify `--fields` option correctly maps CSV columns to BSON field names.

- [ ] **Step 10: Convert `typed_fields.js` and `parse_grace.js`** — Extend or add to existing `typed_fields_test.go`. Verify typed field specifiers (e.g., `name.string()`, `age.int32()`) and `--parseGrace` behavior.

- [ ] **Step 11: Convert `import_write_concern.js`** — `TestImportWriteConcern`: import with `--writeConcern w:2` against a single node; verify appropriate error. Test with `w:1` that it succeeds.

- [ ] **Step 12: Convert topology-specific tests**

  - `import_write_concern_mongos.js` → `TestImportWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Import with write concern against a mongos endpoint, verify it succeeds.
  - `replset.js` → `TestImportReplSet`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Test mongoimport against a replica set URI.
  - `all_primaries_down_error_code.js`, `no_primary_error_code.js` → guard with `testtype.ReplSetTestType`. These require stopping primaries; add as `TestImportNoPrimaryErrorCode` using `mongo.Client` failpoints or by pointing at an unreachable replica set URI.

- [ ] **Step 13: Convert `collections.js`, `import_types.js`, `types.js`, `type_case.js`** as additional cases in the main test file, covering import of multiple collections and all BSON type round-trips.

- [ ] **Step 14: Convert `options.js`** — `TestImportOptionValidation`: verify invalid flag combinations return errors.

- [ ] **Step 15: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongoimport/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 16: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/import/
git add mongoimport/ test/
git commit -m "migration: convert mongoimport JS tests to Go"
```

---

## Chunk 3: mongofiles + mongostat + mongotop

### Task 5: Convert mongofiles JS tests

**JS files:** `test/qa-tests/jstests/files/` (~16 files)
**Go target:** `mongofiles/mongofiles_test.go`

Read `mongofiles/mongofiles_test.go` in full before starting.

The JS tests cover: put, get, list, delete, search, replace commands; `--db`, `--host`, `--port`, `--local`, `--prefix`, `--type`, `--version` options; write concern (standalone and mongos).

The mongofiles API: `mongofiles.MongoFiles{ToolOptions: opts, StorageOptions: storageOpts}`, then `mf.Run()`.

- [ ] **Step 1: Triage** — check which JS tests have Go equivalents.

- [ ] **Step 2: Convert `mongofiles_put.js` and `mongofiles_get.js`** — `TestMongoFilesPutGet`: put a temp file into GridFS, get it back to another temp file, verify byte-for-byte equality.

- [ ] **Step 3: Convert `mongofiles_list.js`** — `TestMongoFilesList`: put several files, list them, verify all appear in output.

- [ ] **Step 4: Convert `mongofiles_delete.js`** — `TestMongoFilesDelete`: put a file, delete it, verify it no longer appears in list.

- [ ] **Step 5: Convert `mongofiles_search.js`** — `TestMongoFilesSearch`: put files with different names, search by prefix, verify only matching names returned.

- [ ] **Step 6: Convert `mongofiles_replace.js`** — `TestMongoFilesReplace`: put a file, replace it with different content, get it back, verify new content.

- [ ] **Step 7: Convert `mongofiles_local.js`** — `TestMongoFilesLocal`: verify `--local` specifies a custom local file path for get/put.

- [ ] **Step 8: Convert `mongofiles_prefix.js`** — `TestMongoFilesPrefix`: verify `--prefix` changes the GridFS bucket name.

- [ ] **Step 9: Convert `mongofiles_type.js`** — `TestMongoFilesType`: verify `--type` sets the MIME type on uploaded files.

- [ ] **Step 10: Convert `mongofiles_invalid.js`** — `TestMongoFilesInvalid`: verify invalid option combinations return errors.

- [ ] **Step 11: Convert `mongofiles_write_concern_mongos.js`** — `TestMongoFilesWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Run a GridFS put against a mongos with an explicit write concern, verify it succeeds.

- [ ] **Step 12: Convert remaining option tests** (`mongofiles_db.js`, `mongofiles_host.js`, `mongofiles_port.js`) as option validation tests.

- [ ] **Step 13: Run tests, delete JS files, commit**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongofiles/... -v -count=1
rm -r test/qa-tests/jstests/files/
git add mongofiles/ test/
git commit -m "migration: convert mongofiles JS tests to Go"
```

---

### Task 6: Convert mongostat JS tests

**JS files:** `test/qa-tests/jstests/stat/` (7 files)
**Go target:** `mongostat/mongostat_test.go`

Read `mongostat/mongostat_test.go` before starting.

The JS tests cover: authentication, broken pipe, custom headers, `--discover` (disabled in JS), `--discover` with shards, header output, `--rowcount`.

mongostat runs as a polling tool; Go tests should run it briefly (1–2 iterations) and verify output format.

- [ ] **Step 1: Triage** — check existing Go coverage.

- [ ] **Step 2: Convert `stat_header.js`** — `TestMongoStatHeader`: run mongostat for 1 second, capture output, verify column headers appear in first line.

- [ ] **Step 3: Convert `stat_rowcount.js`** — `TestMongoStatRowCount`: run mongostat with `--rowcount=3`, verify exactly 3 data rows appear before exit.

- [ ] **Step 4: Convert `stat_custom_headers.js`** — `TestMongoStatCustomHeaders` (if not already covered): run with custom field selection, verify header names match.

- [ ] **Step 5: Convert `stat_auth.js`** — Guard with `testtype.SkipUnlessTestType(t, testtype.AuthTestType)`. Verify mongostat works with `--username`/`--password`.

- [ ] **Step 6: Skip `stat_discover.js`** — it's already disabled in JS (TOOLS-3018 comment). Add `// MIGRATION: SKIP — discover mode disabled per TOOLS-3018`.

- [ ] **Step 7: Convert `stat_discover_shard.js`** — `TestMongoStatDiscoverShard`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Run mongostat with `--discover` against a mongos, verify shard hosts appear in output. Skip `stat_broken_pipe.js` — broken pipe is OS-signal-level behavior.

- [ ] **Step 8: Run tests, delete JS files, commit**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongostat/... -v -count=1
rm -r test/qa-tests/jstests/stat/
git add mongostat/ test/
git commit -m "migration: convert mongostat JS tests to Go"
```

---

### Task 7: Convert mongotop JS tests

**JS files:** `test/qa-tests/jstests/top/` (5 files)
**Go target:** `mongotop/mongotop_test.go` (new file — only `options_test.go` exists today)

The JS tests cover: JSON output format, report output structure, sharded clusters, stress testing, output validation.

- [ ] **Step 1: Create `mongotop/mongotop_test.go`** with package `mongotop`.

- [ ] **Step 2: Convert `mongotop_json.js`** — `TestMongoTopJSONOutput`: run mongotop with `--json` for 1 iteration, parse JSON output, verify expected fields (`ns`, `totalMs`, `readMs`, `writeMs`) are present.

- [ ] **Step 3: Convert `mongotop_reports.js`** — `TestMongoTopReports`: run mongotop for 2 iterations, verify output rows appear for the test database's collections.

- [ ] **Step 4: Convert `mongotop_validation.js`** — `TestMongoTopOutputValidation`: verify column alignment, header row presence, numeric values in output.

- [ ] **Step 5: Convert `mongotop_sharded.js`** — `TestMongoTopSharded`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Run mongotop against a mongos, verify per-shard namespace output. Skip `mongotop_stress.js` — stress tests are not appropriate for a standard integration test run.

- [ ] **Step 6: Run tests, delete JS files, commit**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongotop/... -v -count=1
rm -r test/qa-tests/jstests/top/
git add mongotop/ test/
git commit -m "migration: convert mongotop JS tests to Go"
```

---

## Chunk 4: mongodump

### Task 8: Convert mongodump JS tests

**JS files:** `test/qa-tests/jstests/dump/` (~20 files) and `jstests/txn/active-txn-timestamp.js`
**Go target:** `mongodump/mongodump_qa_test.go` (new file)

Read all of `mongodump/mongodump_test.go` (2537 lines) before starting. Note which scenarios are already covered so you don't duplicate them.

Create `mongodump/mongodump_qa_test.go` with `package mongodump`. It should import the same packages as `mongodump_test.go`.

- [ ] **Step 1: Triage** `dump/` JS files against `mongodump_test.go`. Mark each as SKIP/EXTEND/NEW.

Key existing Go coverage to check against:
- `TestMongoDumpBSON` — general dump/restore roundtrip
- `TestMongoDumpViews` / `TestMongoDumpViewsAsCollections` — view handling
- `TestMongoDumpOplog` — oplog flag
- `TestMongoDumpMetaData` — metadata output
- `TestMongoDump*` for various edge cases

- [ ] **Step 2: Convert `collection_flag_tests.js`** — `TestDumpCollectionFlag`: verify `--collection` requires `--db`, and limits dump to only the specified collection.

- [ ] **Step 3: Convert `db_flag_tests.js`** — `TestDumpDBFlag`: populate two databases, dump only one with `--db`, verify only that DB's BSON files exist in the output.

- [ ] **Step 4: Convert `exclude_collection_tests.js`** — `TestDumpExcludeCollection`: verify `--excludeCollection` flag excludes named collections; verify it cannot be used with `--collection`.

- [ ] **Step 5: Convert `exclude_collections_with_prefix_tests.js`** — `TestDumpExcludeCollectionWithPrefix`: verify `--excludeCollectionsWithPrefix` glob matching.

- [ ] **Step 6: Convert `query_flag_tests.js`** — `TestDumpQueryFlag`: insert 20 docs, dump with `--query '{"x": {"$gt": 10}}'`, verify only matching docs appear in BSON output.

- [ ] **Step 7: Convert `query_extended_json.js`** — `TestDumpQueryExtendedJSON`: dump with an Extended JSON query (e.g., containing `$oid`), verify correct filtering.

- [ ] **Step 8: Convert `out_flag_tests.js`** — `TestDumpOutFlag`: verify `--out` controls output directory path; verify default is `dump/`.

- [ ] **Step 9: Convert `force_table_scan_tests.js`** — `TestDumpForceTableScan`: verify `--forceTableScan` completes without error on a capped collection (where it would normally be required).

- [ ] **Step 10: Convert `dump_views.js`** — If not already covered by `TestMongoDumpViews`, add cases for views created with `--viewOn` and pipeline, verify dump produces correct metadata.

- [ ] **Step 11: Convert `dump_db_users_and_roles_tests.js`** — `TestDumpUsersAndRoles` (under `testtype.AuthTestType`): dump a database that has custom users/roles, verify user/role documents appear in the dump output.

- [ ] **Step 12: Convert `oplog_flag_tests.js`** — If not already covered by `TestMongoDumpOplog`, add sub-cases: oplog file is created, oplog entries from during the dump are captured.

- [ ] **Step 13: Convert `oplog_rename_test.js`** — `TestDumpOplogRename`: perform a collection rename during a dump with `--oplog`, verify the rename is captured in the oplog output.

- [ ] **Step 14: Convert `oplog_rollover_test.js`** — `TestDumpOplogRollover`: verify mongodump exits with an error when the oplog rolls over during a dump.

- [ ] **Step 15: Convert `oplog_admin_sys_version_test.js`** — `TestDumpOplogAdminSysVersion`: verify admin and system collections appear correctly in oplog dump.

- [ ] **Step 16: Convert `options_json.js`** — `TestDumpOptionsJSON`: verify `--out` and other options in combination.

- [ ] **Step 17: Convert `read_preference_and_tags.js`** — `TestDumpReadPreference`: verify `--readPreference` is accepted; if no secondary is available, test that primary mode still works.

- [ ] **Step 18: Convert `no_sharded_secondary_reads.js`** — `TestDumpNoShardedSecondaryReads`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Run mongodump against a mongos with `--readPreference=secondary`; verify it exits with an error (or falls back to primary, depending on expected behavior).

- [ ] **Step 19: Skip the following** — add `// MIGRATION: SKIP` comments:
  - `dump_broken_pipe.js` — OS-level signal handling
  - `dump_server_ko_test.js` — server crash simulation (hard to replicate cleanly in Go)
  - `dumping_dropped_collections.js` — race condition test, not reliably reproducible
  - `version_test.js` — trivially covered by option parsing tests

- [ ] **Step 19: Convert `active-txn-timestamp.js`** (txn/) — Check if it's disabled (JS has a disable comment per TOOLS-2660); if still disabled, add `// MIGRATION: SKIP — disabled per TOOLS-2660`.

- [ ] **Step 20: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongodump/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 21: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/dump/
rm -r test/qa-tests/jstests/txn/
git add mongodump/mongodump_qa_test.go test/
git commit -m "migration: convert mongodump JS tests to Go"
```

---

## Chunk 5: mongorestore

### Task 9: Convert mongorestore core JS tests

**JS files:** `test/qa-tests/jstests/restore/` — first pass: non-oplog, non-auth files (~20 files)
**Go target:** `mongorestore/mongorestore_qa_test.go` (new file)

Read all of `mongorestore/mongorestore_test.go` (3879 lines) and `mongorestore/restore_test.go` before starting. The existing file is comprehensive; many JS tests will be SKIPs.

Create `mongorestore/mongorestore_qa_test.go` with `package mongorestore`.

- [ ] **Step 1: Triage** all `restore/` JS files against existing Go tests.

Key existing Go coverage to check for:
- Archive handling (`mongorestore_archive_test.go`)
- Index restoration (`mongorestore_test.go` — look for TestMongorestore*Index*)
- Transaction handling (`mongorestore_txn_test.go`)
- Oplog replay (`oplog_test.go`)
- Auth tests (`dumprestore_auth_test.go`)
- Metadata tests (`metadata_test.go`)
- Filepath tests (`filepath_test.go`)
- Namespace handling (`ns/ns_test.go`)

- [ ] **Step 2: Convert `indexes.js`** — `TestRestoreIndexes`: dump a collection with text, 2dsphere, sparse, unique, and compound indexes; restore it; verify all indexes are recreated with correct properties.

- [ ] **Step 3: Convert `index_version_roundtrip.js`** — `TestRestoreIndexVersionRoundtrip`: verify index version (v1/v2) is preserved after dump/restore cycle.

- [ ] **Step 4: Convert `keep_index_version.js`** — `TestRestoreKeepIndexVersion`: verify `--keepIndexVersion` flag prevents index version upgrade during restore.

- [ ] **Step 5: Convert `no_index_restore.js`** — `TestRestoreNoIndexRestore`: restore with `--noIndexRestore`, verify no indexes are created (other than `_id`).

- [ ] **Step 6: Convert `collation.js`** — `TestRestoreCollation`: dump a collection with a collation-based index, restore, verify collation settings are preserved.

- [ ] **Step 7: Convert `different_collection.js`** — `TestRestoreDifferentCollection`: restore with `--nsFrom` / `--nsTo`, verify data lands in the renamed collection.

- [ ] **Step 8: Convert `different_db.js`** — `TestRestoreDifferentDB`: restore with database renaming, verify data in target DB.

- [ ] **Step 9: Convert `drop_with_data.js` and `drop_one_collection.js`** — `TestRestoreDrop`: insert extra data into target, restore with `--drop`, verify the pre-existing data is gone.

- [ ] **Step 10: Convert `duplicate_keys.js`** — `TestRestoreDuplicateKeys`: attempt to restore a dump containing documents with duplicate `_id` values; verify correct error behavior.

- [ ] **Step 11: Convert `malformed_bson.js` and `malformed_metadata.js` and `invalid_metadata.js`** — `TestRestoreMalformedInput`: write a temp `.bson` file with corrupted bytes; verify mongorestore returns an appropriate error.

- [ ] **Step 12: Convert `stop_on_error.js`** — `TestRestoreStopOnError`: restore a dump where some documents violate a unique index; with `--stopOnError`, verify restore halts.

- [ ] **Step 13: Convert `blank_collection_bson.js`, `blank_db.js`, `missing_dump.js`** — `TestRestoreEdgeCases`: empty BSON file restores without error; empty DB directory restores without error; missing dump path returns helpful error.

- [ ] **Step 14: Convert `partial_restore.js`** — `TestRestorePartialNS`: use `--ns` to restore only a specific namespace from a multi-collection dump.

- [ ] **Step 15: Convert `namespaces.js`** — `TestRestoreNamespaces`: verify namespace mapping and filtering during restore.

- [ ] **Step 16: Convert `slash_in_collectionname.js`** — `TestRestoreSlashInCollectionName`: dump/restore a collection whose name contains a forward slash.

- [ ] **Step 17: Convert `symlinks.js`** — `TestRestoreSymlinks`: create a dump directory where a BSON file is actually a symlink; verify restore follows or correctly handles it.

- [ ] **Step 18: Convert `large_bulk.js`** — `TestRestoreLargeBulk`: restore a dump with many documents (e.g., 10,000), verify correct count after restore.

- [ ] **Step 19: Convert `write_concern.js`** — `TestRestoreWriteConcern`: restore with explicit write concern, verify it completes.

- [ ] **Step 20: Convert `no_options_restore.js`, `norestore_profile.js`** — `TestRestoreNoOptions`: verify `--noOptionsRestore` prevents collection options (capped size, etc.) from being applied; `--noRestoreProfile` skips the system.profile collection.

- [ ] **Step 21: Convert sharded tests**

  - `sharded_fullrestore.js` → `TestRestoreShardedFullRestore`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Dump a sharded cluster, restore to a fresh sharded cluster (pointed at mongos), verify all collections and their shard distributions are restored correctly.
  - `write_concern_mongos.js` → `TestRestoreWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedTestType)`. Restore with explicit write concern against a mongos, verify it succeeds.

- [ ] **Step 22: Skip the following** with SKIP comments:
  - `archive_stdout.js` — check if already covered in `mongorestore_archive_test.go` first
  - `objcheck_valid_bson.js` — if covered by existing tests

- [ ] **Step 23: Run tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongorestore/... -v -count=1
```

- [ ] **Step 24: Commit**

```bash
git add mongorestore/mongorestore_qa_test.go test/
git commit -m "migration: convert mongorestore core JS tests to Go"
```

---

### Task 10: Convert mongorestore oplog JS tests

**JS files:** `test/qa-tests/jstests/restore/oplog_*.js` (~8 files)
**Go target:** `mongorestore/oplog_test.go` (extend existing) or `mongorestore_qa_test.go`

Read `mongorestore/oplog_test.go` in full before starting.

- [ ] **Step 1: Triage** each `oplog_replay_*.js` file against existing `oplog_test.go`.

- [ ] **Step 2: Convert `oplog_replay_and_limit.js`** — `TestOplogReplayWithLimit`: create an oplog dump with many entries, restore with `--oplogLimit`, verify only entries before the limit are applied.

- [ ] **Step 3: Convert `oplog_replay_conflict.js`** — `TestOplogReplayConflict`: replay an oplog that inserts a document that already exists; verify conflict handling.

- [ ] **Step 4: Convert `oplog_replay_local_rs.js`** — Guard with `testtype.ReplSetTestType`. Verify oplog replay works correctly against a replica set where the local DB is present.

- [ ] **Step 5: Convert `oplog_replay_noop.js`** — `TestOplogReplayNoop`: replay an oplog that contains only no-op entries (type `n`), verify no errors and no data changes.

- [ ] **Step 6: Convert `oplog_replay_no_oplog.js`** — `TestOplogReplayNoOplogFile`: restore with `--oplogReplay` but no oplog.bson file; verify appropriate error message.

- [ ] **Step 7: Convert `oplog_replay_priority_oplog.js`** — `TestOplogReplayPriority`: verify that `oplog.bson` at the root of the dump takes priority over per-DB oplog files.

- [ ] **Step 8: Convert `oplog_replay_size_safety.js`** — `TestOplogReplaySizeSafety`: attempt to replay an oplog entry that exceeds the 16MB BSON document size limit; verify safe error handling.

- [ ] **Step 9: Convert `oplog_replay_specify_file.js`** — `TestOplogReplaySpecifyFile`: use `--oplogFile` to point to a specific oplog dump file rather than the default location.

- [ ] **Step 10: Convert `preserve_oplog_structure_order.js`** — `TestRestorePreserveOplogOrder`: verify that operations within the same transaction are applied in order.

- [ ] **Step 11: Run tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongorestore/... -v -run ".*[Oo]plog.*" -count=1
```

- [ ] **Step 12: Commit**

```bash
git add mongorestore/oplog_test.go mongorestore/mongorestore_qa_test.go test/
git commit -m "migration: convert mongorestore oplog JS tests to Go"
```

---

### Task 11: Convert mongorestore users/roles + auth JS tests

**JS files:** `restore/users_and_roles*.js`, `restore/drop_authenticated_user.js`, `restore/drop_nonexistent_db.js`, `restore/nonempty_temp_users.js`, `restore/extended_json_metadata.js`, `restore/ordered_partial_index.js`, `restore/restore_document_validation.js`
**Go target:** `mongorestore/dumprestore_auth_test.go` (extend) + `mongorestore_qa_test.go`

- [ ] **Step 1: Triage** these files against `dumprestore_auth_test.go`.

- [ ] **Step 2: Convert `users_and_roles.js`** — `TestRestoreUsersAndRoles` (under `testtype.AuthTestType`): dump a database with custom users and roles; restore it to a fresh database; verify users and roles are present.

- [ ] **Step 3: Convert `users_and_roles_admin.js`** — `TestRestoreAdminUsersAndRoles` (under `testtype.AuthTestType`): similar but for admin database users.

- [ ] **Step 4: Convert `users_and_roles_temp_collections.js` / `nonempty_temp_users.js`** — `TestRestoreTempUserCollections`: verify that temporary user collections (`system.new_users`, etc.) are handled correctly during restore.

- [ ] **Step 5: Convert `drop_authenticated_user.js`** — `TestRestoreDropWithAuthUser` (under `testtype.AuthTestType`): restore with `--drop` when the collection contains the currently-authenticated user; verify safe handling.

- [ ] **Step 6: Convert `extended_json_metadata.js`** — `TestRestoreExtendedJSONMetadata`: create a `.metadata.json` file using Extended JSON format for index keys and collection options; verify correct parsing during restore.

- [ ] **Step 7: Convert `restore_document_validation.js`** — `TestRestoreDocumentValidation`: restore with `--bypassDocumentValidation`, verify it bypasses schema validators.

- [ ] **Step 8: Convert `ordered_partial_index.js`** — `TestRestoreOrderedPartialIndex`: dump/restore a collection with a partial index that has a sort key; verify correct restoration.

- [ ] **Step 9: Run tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongorestore/... -v -count=1
```

- [ ] **Step 10: Delete all remaining restore JS files, commit**

```bash
rm -r test/qa-tests/jstests/restore/
git add mongorestore/ test/
git commit -m "migration: convert mongorestore users/roles JS tests to Go"
```

---

## Chunk 6: SSL/TLS + legacy42 + cleanup

### Task 12: Convert SSL/TLS JS tests

**JS files:** `test/qa-tests/jstests/ssl/ssl_with_system_ca.js`, `jstests/tls/tls_with_system_ca.js`
**Go target:** These tests start their own mongod (per resmoke comment). In Go they should use `testtype.SSLTestType`.

These tests verify that the tools can connect to MongoDB using the system's certificate authority store (not a custom CA file). This means connecting with `--tls` and without specifying `--tlsCAFile`.

- [ ] **Step 1: Add `TestExportWithSystemCA`** to `mongoexport/mongoexport_test.go` under `testtype.SkipUnlessTestType(t, testtype.SSLTestType)`. Connect without specifying a CA file; verify export completes.

- [ ] **Step 2: The test needs a mongod running with a TLS cert signed by the system CA** — document in a comment that this test requires `TOOLS_TESTING_MONGOD` to point to a TLS-enabled mongod with a system-trusted certificate.

- [ ] **Step 3: Run the test** manually in a suitable environment to verify; mark as needing manual verification in CI setup notes.

- [ ] **Step 4: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/ssl/
rm -r test/qa-tests/jstests/tls/
git add mongoexport/ test/
git commit -m "migration: convert SSL/TLS JS tests to Go"
```

---

### Task 13: Audit and convert legacy42 JS tests

**JS files:** `test/legacy42/jstests/tool/` (~30 files)

These were removed from the MongoDB server in 4.2. Many will be covered by the tests already converted in Tasks 2–11.

- [ ] **Step 1: Triage every legacy42 file** against the new Go tests written in Tasks 2–11.

  Expected outcome: most are SKIP (covered). A few may test scenarios not yet covered (e.g., `dumpauth.js`, `tool_replset.js`).

- [ ] **Step 2: For any NEW files** — convert using the same patterns as above, adding to the appropriate tool's test file.

  Candidates likely needing conversion:
  - `dumpauth.js` → `mongodump/mongodump_qa_test.go` under `testtype.AuthTestType`
  - `gridfs.js` → check coverage in `mongofiles/mongofiles_test.go`
  - `restorewithauth.js` → check coverage in `mongorestore/dumprestore_auth_test.go`

- [ ] **Step 3: Run the full integration test suite** to confirm all new tests pass

```bash
TOOLS_TESTING_TYPE=integration go test ./... -count=1
```

- [ ] **Step 4: Delete legacy42 directory, commit**

```bash
rm -r test/legacy42/
git add test/ mongodump/ mongorestore/ mongofiles/
git commit -m "migration: convert legacy42 JS tests to Go"
```

---

### Task 14: Remove JS test infrastructure

All JS tests are now gone. Remove the scaffolding.

- [ ] **Step 1: Delete resmoke runner scripts**

```bash
rm scripts/run_qa.sh
rm scripts/run_native_cert_ssl.sh
```

- [ ] **Step 2: Delete the remaining test/qa-tests directory** (helper files, suite configs, etc.)

```bash
rm -r test/qa-tests/
```

  Verify `test/` is now empty (or contains only non-resmoke content):

```bash
ls test/
```

- [ ] **Step 3: Remove tasks from `common.yml`** — find and delete the following task definitions and their references in build variants:
  - `qa-tests-*` tasks
  - `legacy-jstests-*` tasks
  - `native-cert-ssl-*` tasks
  - The `"run qa-tests"` and `"run native-cert-ssl"` functions
  - Any build variant entries that reference only these tasks

  After editing, regenerate and validate the Evergreen config:

```bash
go run build.go sa:evgvalidate
```

- [ ] **Step 4: Update `build.go`** — remove any resmoke-related build targets, if any exist.

- [ ] **Step 5: Run the full test suite one final time**

```bash
TOOLS_TESTING_TYPE=integration go test ./... -count=1
```

Expected: all tests pass, no references to resmoke or JS files remain.

- [ ] **Step 6: Final commit**

```bash
git add -u
git commit -m "migration: remove JS test infrastructure (resmoke, run_qa.sh, common.yml tasks)"
```

---

## Summary of Batches

| Batch | Task | JS files | Go target | Notes |
|---|---|---|---|---|
| 0 | ShardedTestType infra | — | `common/testtype/types.go`, `common.yml` | New test type + CI variant |
| 1 | Audit | All | — | Triage only |
| 2 | bsondump | 7 | `bsondump/bsondump_test.go` | |
| 3 | mongoexport | ~20 | `mongoexport/mongoexport_test.go` | |
| 4 | mongoimport | ~19 | `mongoimport/mongoimport_test.go` | Skip replica set tests |
| 5 | mongofiles | ~16 | `mongofiles/mongofiles_test.go` | |
| 6 | mongostat | 7 | `mongostat/mongostat_test.go` | |
| 7 | mongotop | 5 | `mongotop/mongotop_test.go` (new) | |
| 8 | mongodump | ~20 | `mongodump/mongodump_qa_test.go` (new) | Extend, don't duplicate |
| 9 | mongorestore core | ~20 | `mongorestore/mongorestore_qa_test.go` (new) | |
| 10 | mongorestore oplog | ~8 | `mongorestore/oplog_test.go` | |
| 11 | mongorestore auth | ~7 | `mongorestore/dumprestore_auth_test.go` | |
| 12 | SSL/TLS | 2 | `mongoexport/mongoexport_test.go` | Manual env setup required |
| 13 | legacy42 | ~30 | Various | Mostly SKIP |
| 14 | Cleanup | — | `common.yml`, `build.go`, `scripts/` | Remove infrastructure |

**Consistent SKIPs across all batches:**
- Broken pipe tests — OS-signal behavior, untestable in Go unit/integration tests
- Tests disabled in JS with comments — respect the existing disable rationale

**Sharded/mongos tests** are converted under `testtype.ShardedTestType` (see Task 0), not skipped.
