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
| `jstests/ssl/*.js` | new SSL integration test | Under `testtype.SSLTestType` |
| `jstests/tls/*.js` | new TLS integration test | Under `testtype.SSLTestType` |
| `test/legacy42/jstests/tool/*.js` | Various (see Task 13) | Audit complete; most are duplicates |

---

## Chunk 1: Audit + bsondump

### Task 0: Add ShardedIntegrationTestType infrastructure — COMPLETED

> **COMPLETED** — committed as 5b34c02b. All steps done.

Several JS tests require a sharded cluster (mongos). Rather than skipping them, we introduced a new `ShardedIntegrationTestType` parallel to the existing `ReplSetTestType`.

- [x] **Step 1: Add the constant to `common/testtype/types.go`**

Added `ShardedIntegrationTestType = "TOOLS_TESTING_SHARDED_INTEGRATION"` after the `ReplSetTestType` constant.

- [x] **Step 2: Verify it compiles**

```bash
go build ./common/testtype/...
```

- [x] **Step 3: Add `topology=sharded` support to `buildscript/build.go`**

Added `topology=sharded` as a recognized topology value in the build script.

- [x] **Step 4: Add `"create sharded cluster"` function and `integration-X.Y-sharded` tasks to `common.yml`**

Added new tasks and a function that starts a `ShardingTest` cluster for sharded topology variants.

- [x] **Step 5: Commit**

```bash
git commit -m "testtype: add ShardedIntegrationTestType for sharded cluster integration tests"
```

---

### Task 1: Audit all JS tests against existing Go tests — COMPLETED

> **COMPLETED** — committed as 53346f4c. All JS files annotated. Final counts: 21 SKIP, 111 NEW, 47 EXTEND.

- [x] **Step 1: Read every JS test file** for each tool area. Checked corresponding Go test files for equivalent coverage.

- [x] **Step 2: Record the triage result** by adding a `// MIGRATION:` comment on line 1 of each JS file (SKIP / EXTEND / NEW with rationale).

- [x] **Step 3: Commit the triage annotations**

```bash
git add test/
git commit -m "migration: annotate JS tests with verified Go coverage triage"
```

---

### Task 2: Convert bsondump JS tests

**JS files:** `test/qa-tests/jstests/bson/` (7 files)
**Go target:** `bsondump/bsondump_test.go`

Audit results from the annotations:
- `all_types.js` — NEW
- `all_types_json.js` — NEW
- `bad_files.js` — EXTEND
- `bsondump_broken_pipe.js` — SKIP (OS-signal-level, not testable in Go)
- `bsondump_options.js` — EXTEND
- `deep_nested.js` — NEW
- `output_file.js` — SKIP (already covered by `TestBsondump`)

First, read `bsondump/bsondump_test.go` in full to understand existing coverage.

The bsondump tool is invoked as: `bsondump [options] <file>` — call the library: `bsondump.New(opts)` / `bsondump.Dump(writer)`.

- [ ] **Step 1: Convert `all_types.js`** (NEW) — Add `TestBSONDumpAllTypesDebug`. Insert a document with every BSON type into a collection, dump it to a temp file, run bsondump on it, verify output contains the correct `--type=debug` format type labels and BSON type numbers.

- [ ] **Step 2: Convert `all_types_json.js`** (NEW) — Add `TestBSONDumpAllTypesJSON`. Same setup, verify JSON output mode produces correct Extended JSON for all types including binary, regex, decimal128, etc.

- [ ] **Step 3: Convert `bad_files.js`** (EXTEND) — Add sub-cases to `TestBsondump` in `bsondump_test.go`. Write temp files containing: random bytes, truncated BSON, bad cstring, unsupported type byte. Verify bsondump returns an error or continues with `--objcheck` as appropriate.

- [ ] **Step 4: Convert `deep_nested.js`** (NEW) — Add `TestBSONDumpDeepNested`. Create a deeply nested document (100+ levels), dump it, bsondump it, verify no stack overflow or error.

- [ ] **Step 5: Convert `bsondump_options.js`** (EXTEND) — Add cases to `TestBsondump`. Verify invalid flag combinations return errors; `--type`, `--help`, `--version` flags behave as expected.

- [ ] **Step 6: Skip `output_file.js`** — Already covered by `TestBsondump` (file output variants).

- [ ] **Step 7: Skip `bsondump_broken_pipe.js`** — Broken pipe is OS-signal-level, not testable in Go.

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
rm test/qa-tests/jstests/bson/bsondump_options.js
rm test/qa-tests/jstests/bson/output_file.js
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

**JS files:** `test/qa-tests/jstests/export/` (19 files)
**Go target:** `mongoexport/mongoexport_test.go`

Audit results from the annotations:
- `basic_data.js` — NEW (end-to-end export-then-import round-trip)
- `data_types.js` — NEW (round-trip of diverse BSON types)
- `export_broken_pipe.js` — SKIP (OS-signal-level)
- `export_views.js` — NEW (no Go test creates or exports from a MongoDB view)
- `field_file.js` — NEW
- `fields_csv.js` — EXTEND (csv_test.go covers unit-level CSV but no end-to-end `--fields` with `--csv` against a real server)
- `fields_json.js` — NEW (no Go test verifies `--fields` limits JSON output)
- `force_table_scan.js` — NEW
- `json_array.js` — EXTEND (json_test.go covers basic jsonArray format but not round-trip or import-without-flag failure)
- `limit.js` — NEW (no Go test exercises `--limit`)
- `namespace_validation.js` — NEW
- `nested_fields_csv.js` — EXTEND (add to csv_test.go for nested field handling)
- `no_data.js` — EXTEND (TestMongoExportTOOLS2174 covers empty collection but not `--assertExists`)
- `pretty.js` — NEW
- `query.js` — NEW (no Go test verifies `--query` or `--queryFile` filter export output)
- `slave_ok.js` — NEW (ReplSetTestType)
- `sort_and_skip.js` — NEW (no Go test verifies `--sort` and `--skip`)
- `stdout.js` — NEW (no Go test verifies export writes to stdout)
- `type_case.js` — NEW

Read the existing `mongoexport_test.go` (331 lines — relatively short), `csv_test.go`, and `json_test.go` before starting.

The mongoexport library API: create `mongoexport.MongoExport{Options: opts}`, then call `export.ExpManager.Open()` and `export.Export(writer)`.

- [ ] **Step 1: Convert `basic_data.js`** (NEW) — `TestExportImportBasicRoundtrip`: insert documents, export to buffer, import back, verify document count and values match.

- [ ] **Step 2: Convert `data_types.js`** (NEW) — `TestExportDataTypes`: insert documents with int, float, string, subdoc, array, BinData, ISODate, Timestamp, Regex; verify export contains correct Extended JSON representations.

- [ ] **Step 3: Convert `export_views.js`** (NEW) — `TestExportViews`: create a MongoDB view with a pipeline, export it, verify exported data matches the view's pipeline output.

- [ ] **Step 4: Convert `field_file.js`** (NEW) — `TestExportFieldFile`: verify `--fieldFile` reads field names from a file and limits export output accordingly.

- [ ] **Step 5: Convert `fields_json.js`** (NEW) — `TestExportFieldsJSON`: verify `--fields` limits which fields appear in JSON export output.

- [ ] **Step 6: Convert `fields_csv.js`** (EXTEND) — Add end-to-end case to `mongoexport_test.go`: insert docs, export with `--fields` and `--csv` against a real server, verify only the specified fields appear in CSV output.

- [ ] **Step 7: Convert `nested_fields_csv.js`** (EXTEND) — Add to `csv_test.go`: export with dotted field paths in `--fields`, verify CSV flattening behavior.

- [ ] **Step 8: Convert `json_array.js`** (EXTEND) — Add to `mongoexport_test.go`: verify `--jsonArray` wraps output in `[...]`, and that importing without `--jsonArray` fails on that output.

- [ ] **Step 9: Convert `force_table_scan.js`** (NEW) — `TestExportForceTableScan`: verify `--forceTableScan` flag doesn't break basic export.

- [ ] **Step 10: Convert `limit.js`** (NEW) — `TestExportLimit`: insert 20 docs, export with `--limit 5`, verify exactly 5 docs in output.

- [ ] **Step 11: Convert `namespace_validation.js`** (NEW) — `TestExportNamespaceValidation`: verify that invalid namespace combinations (no `--db`, `--collection` without `--db`, etc.) return appropriate errors.

- [ ] **Step 12: Convert `no_data.js`** (EXTEND) — Add to `TestMongoExportTOOLS2174` or a new case: verify `--assertExists` flag returns an error for a collection that does not exist.

- [ ] **Step 13: Convert `pretty.js`** (NEW) — `TestExportPretty`: verify `--pretty` flag produces indented JSON output.

- [ ] **Step 14: Convert `query.js`** (NEW) — `TestExportQuery`: insert 10 docs, export with `--query '{"x": {"$gt": 5}}'` and with `--queryFile`, verify only matching docs appear.

- [ ] **Step 15: Convert `sort_and_skip.js`** (NEW) — `TestExportSortAndSkip`: insert ordered docs, export with `--sort` and `--skip`, verify ordering and offset.

- [ ] **Step 16: Convert `stdout.js`** (NEW) — `TestExportStdout`: verify export writes correct JSON to stdout when no `--out` is specified.

- [ ] **Step 17: Convert `type_case.js`** (NEW) — `TestExportTypeCase`: verify type name case insensitivity in export output format selection.

- [ ] **Step 18: Convert `slave_ok.js`** (NEW) — `TestExportSlaveOk`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Verify export works against a replica set with secondary read preference.

- [ ] **Step 19: Skip `export_broken_pipe.js`** — OS-signal-level, not testable in Go.

- [ ] **Step 20: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongoexport/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 21: Delete the JS files, commit**

```bash
rm -r test/qa-tests/jstests/export/
git add mongoexport/mongoexport_test.go mongoexport/csv_test.go mongoexport/json_test.go test/
git commit -m "migration: convert mongoexport JS tests to Go"
```

---

### Task 4: Convert mongoimport JS tests

**JS files:** `test/qa-tests/jstests/import/` (20 files)
**Go target:** `mongoimport/mongoimport_test.go`

Audit results from the annotations:
- `all_primaries_down_error_code.js` — NEW (ShardedIntegrationTestType)
- `boolean_type.js` — NEW (Boolean() objects round-trip)
- `collections.js` — EXTEND (basic file-to-collection derivation covered but not multi-dot filenames, positional args, or `--db` with positional arg)
- `decimal128.js` — NEW (no Go test verifies Decimal128 round-trip through Extended JSON)
- `drop.js` — SKIP (covered by `mongoimport_test.go`)
- `fields.js` — EXTEND (covers `--fields` and `--ignoreBlanks` for CSV but not `--fieldFile`, nested dotted field names, or extra fields beyond header end-to-end)
- `import_document_validation.js` — NEW (no Go test for validated collection + import rejection + `--bypassDocumentValidation`)
- `import_types.js` — NEW (no Go test imports legacy Extended JSON with all BSON types and verifies `$type`)
- `import_write_concern.js` — NEW (ReplSetTestType)
- `import_write_concern_mongos.js` — NEW (ShardedIntegrationTestType)
- `mode.js` — EXTEND (upsert and delete covered but not merge, compound `--upsertFields`, or legacy `--upsert` flag)
- `mode_upsert_id_subdoc.js` — EXTEND (add upsert-with-ID-in-subdoc case to `mongoimport_test.go`)
- `no_primary_error_code.js` — NEW (ShardedIntegrationTestType)
- `options.js` — EXTEND (some option validation covered but not invalid DB/collection names, `--jsonArray` with non-array input, type mismatches, or conflicting positional args)
- `parse_grace.js` — NEW
- `replset.js` — NEW (ReplSetTestType)
- `stoponerror.js` — SKIP (covered by `mongoimport_test.go`)
- `type_case.js` — NEW
- `typed_fields.js` — EXTEND (typed_fields_test.go covers header parsing at unit level but no end-to-end `--columnsHaveTypes` verifying actual database contents)
- `types.js` — NEW (round-trip of all BSON types including BinData, Boolean, Array, NumberLong, MinKey, MaxKey, ISODate, DBRef, etc.)

Read `mongoimport_test.go` (1525 lines), `csv_test.go`, `json_test.go`, `tsv_test.go`, and `typed_fields_test.go` before starting.

The mongoimport API: `mongoimport.MongoImport{ToolOptions: opts, IngestOptions: ingestOpts, InputOptions: inputOpts}`, then `mi.ImportDocuments()`.

- [ ] **Step 1: Convert `boolean_type.js`** (NEW) — `TestImportBooleanType`: import a JSON file with `Boolean()` objects, verify they round-trip as BSON booleans.

- [ ] **Step 2: Convert `collections.js`** (EXTEND) — Add cases to `TestMongoImportValidateSettings` covering: multi-dot filenames, positional arguments for collection name, `--db` combined with positional arg.

- [ ] **Step 3: Convert `decimal128.js`** (NEW) — `TestImportDecimal128`: export a document with a Decimal128 field as Extended JSON, import it back, verify the field is stored as BSON Decimal128.

- [ ] **Step 4: Convert `fields.js`** (EXTEND) — Add to existing field tests: `--fieldFile` option, nested dotted field names, and CSV rows with more fields than the header.

- [ ] **Step 5: Convert `import_document_validation.js`** (NEW) — `TestImportDocumentValidation`: create a collection with a `$jsonSchema` validator, import documents that violate it, verify rejection; test `--bypassDocumentValidation` and `--stopOnError` with validation errors.

- [ ] **Step 6: Convert `import_types.js`** (NEW) — `TestImportTypes`: import a legacy Extended JSON file containing all BSON types, verify each field's `$type` in the database.

- [ ] **Step 7: Convert `mode.js`** (EXTEND) — Add missing cases to existing mode tests: `--mode=merge` (preserving pre-existing fields), compound `--upsertFields` with non-`_id` fields, and legacy `--upsert` flag.

- [ ] **Step 8: Convert `mode_upsert_id_subdoc.js`** (EXTEND) — Add case to `mongoimport_test.go`: import documents whose `_id` is a subdocument, verify upsert uses the full subdoc as the key.

- [ ] **Step 9: Convert `options.js`** (EXTEND) — Add to `TestMongoImportValidateSettings`: invalid DB/collection names, `--jsonArray` with non-array input, type mismatches, conflicting positional args.

- [ ] **Step 10: Convert `parse_grace.js`** (NEW) — `TestImportParseGrace`: verify `--parseGrace` behavior (stop, skipRow, skipField) when fields contain type conversion errors.

- [ ] **Step 11: Convert `type_case.js`** (NEW) — `TestImportTypeCase`: verify type name case insensitivity when specifying type formats.

- [ ] **Step 12: Convert `typed_fields.js`** (EXTEND) — Add end-to-end test to `typed_fields_test.go` (or `mongoimport_test.go`): import a CSV with `--columnsHaveTypes`, verify actual database contents match the declared types.

- [ ] **Step 13: Convert `types.js`** (NEW) — `TestImportAllBSONTypes`: verify round-trip export-then-import preserves all BSON types (BinData, Boolean, Array, embedded doc, NumberLong, MinKey, MaxKey, ISODate, DBRef, etc.).

- [ ] **Step 14: Convert `import_write_concern.js`** (NEW) — `TestImportWriteConcern`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Import with `--writeConcern w:2` against a single node; verify appropriate error. Test with `w:1` that it succeeds.

- [ ] **Step 15: Convert `replset.js`** (NEW) — `TestImportReplSet`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Test mongoimport against a replica set URI.

- [ ] **Step 16: Convert sharded topology tests** (NEW)

  - `import_write_concern_mongos.js` → `TestImportWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Import with write concern against a mongos endpoint, verify it succeeds.
  - `all_primaries_down_error_code.js` → `TestImportAllPrimariesDownErrorCode`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`.
  - `no_primary_error_code.js` → `TestImportNoPrimaryErrorCode`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`.

- [ ] **Step 17: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongoimport/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 18: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/import/
git add mongoimport/ test/
git commit -m "migration: convert mongoimport JS tests to Go"
```

---

## Chunk 3: mongofiles + mongostat + mongotop

### Task 5: Convert mongofiles JS tests

**JS files:** `test/qa-tests/jstests/files/` (16 files)
**Go target:** `mongofiles/mongofiles_test.go`

Audit results from the annotations:
- `mongofiles_db.js` — NEW (no Go test verifies `--db` routes files to the correct non-default database)
- `mongofiles_delete.js` — SKIP (covered by `mongofiles_test.go`)
- `mongofiles_get.js` — EXTEND (Go tests cover get and get_id to file but not get-to-stdout via `--local -`)
- `mongofiles_host.js` — NEW (no Go test validates `--host` behavior)
- `mongofiles_invalid.js` — EXTEND (`TestValidArguments` covers invalid command but not invalid CLI option `--invalid`)
- `mongofiles_list.js` — SKIP (covered by `mongofiles_test.go`)
- `mongofiles_local.js` — EXTEND (Go tests cover `--local` for get but not for put, empty `--local` string, or nonexistent file)
- `mongofiles_port.js` — NEW (no Go test validates `--port` behavior)
- `mongofiles_prefix.js` — NEW (no Go test validates `--prefix` routing to custom GridFS collection)
- `mongofiles_put.js` — EXTEND (Go tests cover basic put and content verification but not large multi-chunk files 40MB+ or put-of-a-directory-fails)
- `mongofiles_replace.js` — NEW
- `mongofiles_search.js` — NEW
- `mongofiles_type.js` — NEW
- `mongofiles_version.js` — SKIP (standard `--version` flag)
- `mongofiles_write_concern.js` — NEW (ReplSetTestType)
- `mongofiles_write_concern_mongos.js` — NEW (ShardedIntegrationTestType)

Read `mongofiles/mongofiles_test.go` in full before starting.

The mongofiles API: `mongofiles.MongoFiles{ToolOptions: opts, StorageOptions: storageOpts}`, then `mf.Run()`.

- [ ] **Step 1: Convert `mongofiles_get.js`** (EXTEND) — Add case to existing get tests: get-to-stdout by passing `--local -`, verify output matches file content.

- [ ] **Step 2: Convert `mongofiles_invalid.js`** (EXTEND) — Add case to `TestValidArguments`: verify passing an invalid CLI option (`--invalid`) returns an appropriate error.

- [ ] **Step 3: Convert `mongofiles_local.js`** (EXTEND) — Add cases: `--local` for put specifying a custom path; empty `--local` string fails; `--local` pointing to a nonexistent file for put fails.

- [ ] **Step 4: Convert `mongofiles_put.js`** (EXTEND) — Add case: put a large multi-chunk file (40MB+) and verify all chunks are stored; verify put-of-a-directory fails.

- [ ] **Step 5: Convert `mongofiles_db.js`** (NEW) — `TestMongoFilesDB`: put a file with `--db myfiles_test`, verify it appears in `myfiles_test.fs.files` and not in the default `test.fs.files`.

- [ ] **Step 6: Convert `mongofiles_host.js`** (NEW) — `TestMongoFilesHost`: verify valid host succeeds, invalid/unreachable host fails with connection error.

- [ ] **Step 7: Convert `mongofiles_port.js`** (NEW) — `TestMongoFilesPort`: verify valid port succeeds, wrong or non-numeric port fails with appropriate error.

- [ ] **Step 8: Convert `mongofiles_prefix.js`** (NEW) — `TestMongoFilesPrefix`: put a file with `--prefix mygridfs`, verify the file appears in `mygridfs.files` collection.

- [ ] **Step 9: Convert `mongofiles_replace.js`** (NEW) — `TestMongoFilesReplace`: put a file, replace it with different content, get it back, verify new content.

- [ ] **Step 10: Convert `mongofiles_search.js`** (NEW) — `TestMongoFilesSearch`: put files with different names, search by prefix, verify only matching names returned.

- [ ] **Step 11: Convert `mongofiles_type.js`** (NEW) — `TestMongoFilesType`: put a file with `--type image/png`, verify the contentType field in GridFS metadata.

- [ ] **Step 12: Convert `mongofiles_write_concern.js`** (NEW) — `TestMongoFilesWriteConcern`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Run a GridFS put with explicit write concern, verify it succeeds.

- [ ] **Step 13: Convert `mongofiles_write_concern_mongos.js`** (NEW) — `TestMongoFilesWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run a GridFS put against a mongos with explicit write concern, verify it succeeds.

- [ ] **Step 14: Run tests, delete JS files, commit**

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

Audit results from the annotations:
- `stat_auth.js` — NEW (ShardedIntegrationTestType per annotation)
- `stat_broken_pipe.js` — SKIP (OS-signal-level)
- `stat_custom_headers.js` — NEW (ShardedIntegrationTestType)
- `stat_discover.js` — SKIP (disabled per TOOLS-3018)
- `stat_discover_shard.js` — NEW (ShardedIntegrationTestType)
- `stat_header.js` — NEW (ShardedIntegrationTestType)
- `stat_rowcount.js` — NEW (ShardedIntegrationTestType)

Read `mongostat/mongostat_test.go` before starting.

mongostat runs as a polling tool; Go tests should run it briefly (1–2 iterations) and verify output format.

- [ ] **Step 1: Convert `stat_header.js`** (NEW) — `TestMongoStatHeader`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run mongostat for 1 second, capture output, verify column headers appear in first line.

- [ ] **Step 2: Convert `stat_rowcount.js`** (NEW) — `TestMongoStatRowCount`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run mongostat with `--rowcount=3`, verify exactly 3 data rows appear before exit.

- [ ] **Step 3: Convert `stat_custom_headers.js`** (NEW) — `TestMongoStatCustomHeaders`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run with custom field selection, verify header names match.

- [ ] **Step 4: Convert `stat_auth.js`** (NEW) — `TestMongoStatAuth`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Verify mongostat works with `--username`/`--password`.

- [ ] **Step 5: Convert `stat_discover_shard.js`** (NEW) — `TestMongoStatDiscoverShard`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run mongostat with `--discover` against a mongos, verify shard hosts appear in output.

- [ ] **Step 6: Skip `stat_discover.js`** — already disabled in JS per TOOLS-3018.

- [ ] **Step 7: Skip `stat_broken_pipe.js`** — broken pipe is OS-signal-level behavior.

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

Audit results from the annotations:
- `mongotop_json.js` — NEW (options_test.go only tests argument parsing; no Go test runs mongotop and verifies JSON output)
- `mongotop_reports.js` — NEW (no Go test runs mongotop and verifies namespace activity reporting)
- `mongotop_sharded.js` — NEW (ShardedIntegrationTestType)
- `mongotop_stress.js` — NEW (note: stress tests should not be part of standard integration run)
- `mongotop_validation.js` — NEW (options_test.go only tests positional argument parsing; no Go test covers invalid port, invalid rowcount, negative sleep time errors)

- [ ] **Step 1: Create `mongotop/mongotop_test.go`** with package `mongotop`.

- [ ] **Step 2: Convert `mongotop_json.js`** (NEW) — `TestMongoTopJSONOutput`: run mongotop with `--json` for 1 iteration, parse JSON output, verify expected fields (`ns`, `totalMs`, `readMs`, `writeMs`) are present.

- [ ] **Step 3: Convert `mongotop_reports.js`** (NEW) — `TestMongoTopReports`: run mongotop for 2 iterations, verify output rows appear for the test database's collections.

- [ ] **Step 4: Convert `mongotop_validation.js`** (NEW) — `TestMongoTopValidation`: verify that invalid port, invalid rowcount value, and negative sleep time each return appropriate errors.

- [ ] **Step 5: Convert `mongotop_sharded.js`** (NEW) — `TestMongoTopSharded`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Run mongotop against a mongos, verify per-shard namespace output.

- [ ] **Step 6: Skip `mongotop_stress.js`** — stress tests are not appropriate for a standard integration test run. If desired, add as a separate stress test binary later.

- [ ] **Step 7: Run tests, delete JS files, commit**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongotop/... -v -count=1
rm -r test/qa-tests/jstests/top/
git add mongotop/ test/
git commit -m "migration: convert mongotop JS tests to Go"
```

---

## Chunk 4: mongodump

### Task 8: Convert mongodump JS tests

**JS files:** `test/qa-tests/jstests/dump/` (~22 files) and `jstests/txn/active-txn-timestamp.js`
**Go target:** `mongodump/mongodump_qa_test.go` (new file)

Audit results from the annotations:
- `collection_flag_tests.js` — NEW (no Go test verifies `--collection` dumps only the named collection)
- `db_flag_tests.js` — NEW (no Go test inserts into two DBs, dumps one with `--db`, and verifies exclusion)
- `dump_broken_pipe.js` — SKIP (OS-signal-level)
- `dump_db_users_and_roles_tests.js` — EXTEND (main scenario covered but missing "no users exist" error and `--dumpDbUsersAndRoles` without `--db` error; goes in `mongorestore/dumprestore_auth_test.go`)
- `dumping_dropped_collections.js` — SKIP (test was already disabled per TOOLS-3019)
- `dump_server_ko_test.js` — NEW (no Go coverage)
- `dump_views.js` — EXTEND (`TestMongoDumpViews` checks metadata files but no Go test restores and verifies view data round-trip or `--viewsAsCollections` behavior)
- `exclude_collections_with_prefix_tests.js` — NEW (no Go coverage)
- `exclude_collection_tests.js` — NEW (no Go coverage)
- `force_table_scan_tests.js` — NEW (no Go coverage)
- `no_sharded_secondary_reads.js` — SKIP (disabled per TOOLS-2661)
- `oplog_admin_sys_version_test.js` — NEW (no Go coverage; ReplSetTestType; goes in `oplog_dump_test.go`)
- `oplog_flag_tests.js` — EXTEND (add to `TestMongoDumpOplog`)
- `oplog_rename_test.js` — NEW (ReplSetTestType; goes in `oplog_dump_test.go`)
- `oplog_rollover_test.js` — NEW (ReplSetTestType; goes in `oplog_dump_test.go`)
- `options_json.js` — NEW (no Go coverage)
- `out_flag_tests.js` — NEW (no Go test covers `--out -` error cases or `--out` to custom directory with restore)
- `query_extended_json.js` — NEW (no Go test verifies Extended JSON types in `--query`)
- `query_flag_tests.js` — EXTEND (`TestMongoDumpOrderedQuery` and `TestMongoDumpBSON` cover success paths but missing error cases: without `--db`, without `--collection`, nonexistent `--queryFile`)
- `read_preference_and_tags.js` — NEW (ReplSetTestType; no Go coverage)
- `version_test.js` — SKIP (standard CLI `--version` flag)
- `active-txn-timestamp.js` (txn/) — SKIP (disabled per TOOLS-2660)

Read all of `mongodump/mongodump_test.go` (2537 lines) before starting. Create `mongodump/mongodump_qa_test.go` with `package mongodump`.

- [ ] **Step 1: Convert `collection_flag_tests.js`** (NEW) — `TestDumpCollectionFlag`: verify `--collection` requires `--db`, and limits dump to only the specified collection excluding other collections.

- [ ] **Step 2: Convert `db_flag_tests.js`** (NEW) — `TestDumpDBFlag`: insert into two databases, dump only one with `--db`, verify only that DB's BSON files exist in the output and the other DB is absent.

- [ ] **Step 3: Convert `exclude_collection_tests.js`** (NEW) — `TestDumpExcludeCollection`: verify `--excludeCollection` flag excludes named collections; verify it cannot be used with `--collection`.

- [ ] **Step 4: Convert `exclude_collections_with_prefix_tests.js`** (NEW) — `TestDumpExcludeCollectionWithPrefix`: verify `--excludeCollectionsWithPrefix` glob matching excludes all matching collections.

- [ ] **Step 5: Convert `query_flag_tests.js`** (EXTEND) — Add error cases to existing query tests: `--query` without `--db`, `--query` without `--collection`, and nonexistent `--queryFile`.

- [ ] **Step 6: Convert `query_extended_json.js`** (NEW) — `TestDumpQueryExtendedJSON`: dump with Extended JSON query types (`$date`, `$regex`, `$oid`, `$minKey`, `$maxKey`), verify correct filtering.

- [ ] **Step 7: Convert `out_flag_tests.js`** (NEW) — `TestDumpOutFlag`: verify `--out -` fails without `--db`/`--collection`; verify `--out` to a custom directory combined with restore produces correct results.

- [ ] **Step 8: Convert `force_table_scan_tests.js`** (NEW) — `TestDumpForceTableScan`: verify `--forceTableScan` completes without error.

- [ ] **Step 9: Convert `dump_views.js`** (EXTEND) — Add cases to dump views test: restore view and verify data matches the view's pipeline output; test `--viewsAsCollections` behavior.

- [ ] **Step 10: Convert `dump_db_users_and_roles_tests.js`** (EXTEND) — In `mongorestore/dumprestore_auth_test.go`, add: "no users exist" error case and `--dumpDbUsersAndRoles` without `--db` error case.

- [ ] **Step 11: Convert `oplog_flag_tests.js`** (EXTEND) — Add sub-cases to `TestMongoDumpOplog`: verify oplog file is created; verify oplog entries from during the dump are captured.

- [ ] **Step 12: Convert `dump_server_ko_test.js`** (NEW) — `TestDumpServerKO`: verify mongodump handles a server that becomes unavailable during the dump with an appropriate error. Guard carefully against flakiness.

- [ ] **Step 13: Convert `options_json.js`** (NEW) — `TestDumpOptionsJSON`: verify `--out` and other options in combination work correctly.

- [ ] **Step 14: Convert `oplog_rename_test.js`** (NEW) — Guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Goes in `oplog_dump_test.go` (create if needed). Perform a collection rename during a dump with `--oplog`, verify the rename is captured.

- [ ] **Step 15: Convert `oplog_rollover_test.js`** (NEW) — Guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Verify mongodump exits with an error when the oplog rolls over during a dump.

- [ ] **Step 16: Convert `oplog_admin_sys_version_test.js`** (NEW) — Guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Verify admin and system collections appear correctly in oplog dump.

- [ ] **Step 17: Convert `read_preference_and_tags.js`** (NEW) — Guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Verify `--readPreference` is accepted and dump completes.

- [ ] **Step 18: Skip the following:**
  - `dump_broken_pipe.js` — OS-level signal handling
  - `dumping_dropped_collections.js` — disabled per TOOLS-3019
  - `no_sharded_secondary_reads.js` — disabled per TOOLS-2661
  - `version_test.js` — trivially covered by option parsing tests
  - `active-txn-timestamp.js` — disabled per TOOLS-2660

- [ ] **Step 19: Run the new tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongodump/... -v -count=1
```

Expected: all PASS.

- [ ] **Step 20: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/dump/
rm -r test/qa-tests/jstests/txn/
git add mongodump/mongodump_qa_test.go test/
git commit -m "migration: convert mongodump JS tests to Go"
```

---

## Chunk 5: mongorestore

### Task 9: Convert mongorestore core JS tests

**JS files:** `test/qa-tests/jstests/restore/` — non-oplog, non-auth files
**Go target:** `mongorestore/mongorestore_qa_test.go` (new file)

Audit results (non-oplog, non-auth, non-sharded subset):
- `archive_stdout.js` — EXTEND (pipe-based dump `--archive | restore --archive` with special characters not covered)
- `bad_options.js` — NEW (ReplSetTestType)
- `blank_collection_bson.js` — NEW (no Go test restores from blank .bson file with missing/blank metadata)
- `blank_db.js` — NEW (no Go test restores from an empty directory with `--db`)
- `collation.js` — EXTEND (`TestIndexGetsSimpleCollation` covers index collation but not collection-level collation round-trip)
- `different_collection.js` — SKIP (rename collection on restore covered by `mongorestore_test.go`)
- `different_db.js` — EXTEND (`--nsFrom`/`--nsTo` covered but `--db` dest path restoring a subdirectory is not)
- `drop_nonexistent_db.js` — NEW (no Go test verifies `--drop` on a nonexistent DB succeeds without error)
- `drop_one_collection.js` — NEW (no Go test verifies `--drop --db --collection` only drops the specified collection)
- `drop_with_data.js` — EXTEND (no Go test specifically inserts different pre-existing data and verifies it is fully replaced)
- `duplicate_keys.js` — EXTEND (`TestMongorestoreMIOSOE` covers dup key with stopOnError but not batch size iteration behavior)
- `indexes.js` — EXTEND (`TestCreateIndexes` tests hashed indexes but no Go test creates/round-trips sparse, unique, compound, text, and 2dsphere indexes)
- `index_version_roundtrip.js` — NEW (no Go test does round-trip with `--keepIndexVersion` verifying version values)
- `invalid_dump_target.js` — NEW (no Go test passes invalid targets and checks errors)
- `invalid_metadata.js` — NEW (no Go test verifies restore with invalid indexes in metadata.json fails)
- `keep_index_version.js` — NEW (no Go test covers `--keepIndexVersion`)
- `large_bulk.js` — NEW (no Go test creates 32 x ~1MB documents to verify bulk API respects 16MB BSON limit)
- `malformed_bson.js` — NEW (no Go test restores malformed BSON and verifies failure)
- `malformed_metadata.js` — NEW (no Go test restores with malformed metadata.json and verifies failure)
- `missing_dump.js` — NEW (no Go test passes nonexistent paths and verifies error)
- `multiple_dbs.js` — EXTEND (`TestMongorestore` restores from testdata but does not explicitly verify per-collection document counts)
- `namespaces.js` — EXTEND (`--nsExclude`, `--nsInclude`, `--nsFrom`/`--nsTo` covered but not `--excludeCollectionsWithPrefix` or complex pattern variables)
- `no_index_restore.js` — EXTEND (Go tests use `--noIndexRestore` only in timeseries context; no Go test verifies general case)
- `nonempty_temp_users.js` — EXTEND (`TestRestoreUsersOrRoles` covers cleanup but not the case where `admin.tempusers` already contains data before restore)
- `no_options_restore.js` — EXTEND (Go tests use `--noOptionsRestore` only in timeseries context; no Go test verifies capped → non-capped or validator stripping)
- `norestore_profile.js` — NEW (no Go test specifically verifies system.profile is not restored)
- `objcheck_valid_bson.js` — EXTEND (no Go test explicitly passes `--objcheck` and verifies the flag is accepted)
- `partial_restore.js` — EXTEND (`--nsInclude` for single-collection restore covered but not `--db` path with subdirectory target)
- `restore_document_validation.js` — NEW (no Go test for validated collection + restore rejection + `--bypassDocumentValidation`)
- `slash_in_collectionname.js` — SKIP (covered by `mongorestore_test.go`)
- `stop_on_error.js` — SKIP (covered by `mongorestore_test.go`)
- `symlinks.js` — NEW (no Go test exercises restoring from dump directory with symlinked files)
- `users_and_roles_temp_collections.js` — EXTEND (`TestRestoreUsersOrRoles` covers cleanup but not `--tempUsersColl` and `--tempRolesColl` custom temp collection name options)
- `write_concern.js` — NEW (ReplSetTestType)

Read all of `mongorestore/mongorestore_test.go` (3879 lines) and `mongorestore/restore_test.go` before starting. Create `mongorestore/mongorestore_qa_test.go` with `package mongorestore`.

- [ ] **Step 1: Convert `archive_stdout.js`** (EXTEND) — Add to `mongorestore_archive_test.go` or `mongorestore_qa_test.go`: pipe-based `dump --archive | restore --archive` with special characters in collection names.

- [ ] **Step 2: Convert `blank_collection_bson.js`** (NEW) — `TestRestoreBlankCollectionBSON`: restore from a blank `.bson` file with missing or blank metadata; verify it succeeds without error.

- [ ] **Step 3: Convert `blank_db.js`** (NEW) — `TestRestoreBlankDB`: restore from an empty directory with `--db`; verify it completes without error.

- [ ] **Step 4: Convert `collation.js`** (EXTEND) — Add to restore tests: dump a collection with a collection-level collation, restore it, verify the collation setting is preserved on the restored collection.

- [ ] **Step 5: Convert `different_db.js`** (EXTEND) — Add case: restore a subdirectory dump using `--db` to target a different destination database, verify data lands correctly.

- [ ] **Step 6: Convert `drop_nonexistent_db.js`** (NEW) — `TestRestoreDropNonexistentDB`: pass `--drop` when the target database does not exist; verify success without error.

- [ ] **Step 7: Convert `drop_one_collection.js`** (NEW) — `TestRestoreDropOneCollection`: insert data into multiple collections, then restore with `--drop --db --collection` for only one of them; verify only the specified collection is dropped and replaced.

- [ ] **Step 8: Convert `drop_with_data.js`** (EXTEND) — Add case: insert different pre-existing data into target, restore with `--drop`, verify the pre-existing data is fully replaced by the restored data.

- [ ] **Step 9: Convert `duplicate_keys.js`** (EXTEND) — Add case to existing dup key tests: verify batch size iteration behavior when multiple batches contain duplicate keys.

- [ ] **Step 10: Convert `indexes.js`** (EXTEND) — Add cases to index restoration tests: round-trip sparse, unique, compound, text, and 2dsphere indexes; verify each is correctly recreated.

- [ ] **Step 11: Convert `index_version_roundtrip.js`** (NEW) — `TestRestoreIndexVersionRoundtrip`: dump and restore with `--keepIndexVersion`; verify index version values are preserved.

- [ ] **Step 12: Convert `invalid_dump_target.js`** (NEW) — `TestRestoreInvalidDumpTarget`: pass invalid targets (file instead of directory, file with `--db`, directory with `--collection`) and check for appropriate errors.

- [ ] **Step 13: Convert `invalid_metadata.js`** (NEW) — `TestRestoreInvalidMetadata`: restore a collection whose `metadata.json` contains invalid index definitions; verify failure.

- [ ] **Step 14: Convert `keep_index_version.js`** (NEW) — `TestRestoreKeepIndexVersion`: verify `--keepIndexVersion` flag prevents index version upgrade during restore. Note: targets behavior on older server versions.

- [ ] **Step 15: Convert `large_bulk.js`** (NEW) — `TestRestoreLargeBulk`: create 32 documents each ~1MB, dump and restore them; verify the bulk API respects the 16MB BSON document limit and all documents restore correctly.

- [ ] **Step 16: Convert `malformed_bson.js`** (NEW) — `TestRestoreMalformedBSON`: write a temp `.bson` file with corrupted bytes; verify mongorestore returns an appropriate error.

- [ ] **Step 17: Convert `malformed_metadata.js`** (NEW) — `TestRestoreMalformedMetadata`: restore with a syntactically invalid `metadata.json`; verify failure with a clear error.

- [ ] **Step 18: Convert `missing_dump.js`** (NEW) — `TestRestoreMissingDump`: pass nonexistent paths (directory, with `--db`, with `--collection`); verify appropriate error messages.

- [ ] **Step 19: Convert `multiple_dbs.js`** (EXTEND) — Add explicit per-collection document count assertions after restoring from a multi-DB dump (`testdata/testdirs` or equivalent).

- [ ] **Step 20: Convert `namespaces.js`** (EXTEND) — Add cases: `--excludeCollectionsWithPrefix` behavior and complex `--nsInclude`/`--nsExclude` pattern variables.

- [ ] **Step 21: Convert `no_index_restore.js`** (EXTEND) — Add case on a normal (non-timeseries) collection: restore with `--noIndexRestore`, verify no indexes are created beyond `_id`.

- [ ] **Step 22: Convert `no_options_restore.js`** (EXTEND) — Add cases: dump a capped collection and a collection with a validator, restore with `--noOptionsRestore`, verify the collection is non-capped and validator is stripped.

- [ ] **Step 23: Convert `norestore_profile.js`** (NEW) — `TestRestoreNorestoreProfile`: dump a database that includes system.profile, restore it, verify system.profile is not present in the restore target.

- [ ] **Step 24: Convert `objcheck_valid_bson.js`** (EXTEND) — Add case: restore valid BSON with `--objcheck`, verify the flag is accepted and restore succeeds.

- [ ] **Step 25: Convert `partial_restore.js`** (EXTEND) — Add case: restore a subdirectory using `--db` path targeting, verify only that subdirectory's collections are restored.

- [ ] **Step 26: Convert `restore_document_validation.js`** (NEW) — `TestRestoreDocumentValidation`: create a validated collection, restore into it, verify rejection; test `--bypassDocumentValidation`; test `--stopOnError` with validation errors; test `--maintainInsertionOrder` with validation errors.

- [ ] **Step 27: Convert `symlinks.js`** (NEW) — `TestRestoreSymlinks`: create a dump directory where a `.bson` file is a symlink to actual data; verify restore follows the symlink correctly.

- [ ] **Step 28: Convert `users_and_roles_temp_collections.js`** (EXTEND) — Add cases: verify `--tempUsersColl` and `--tempRolesColl` options correctly name the custom temp collections during restore.

- [ ] **Step 29: Convert `write_concern.js`** (NEW) — `TestRestoreWriteConcern`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Restore with explicit write concern; verify it completes.

- [ ] **Step 30: Convert `bad_options.js`** (NEW) — `TestRestoreBadOptions`: guard with `testtype.SkipUnlessTestType(t, testtype.ReplSetTestType)`. Verify invalid option combinations produce appropriate errors.

- [ ] **Step 31: Run tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongorestore/... -v -count=1
```

- [ ] **Step 32: Commit**

```bash
git add mongorestore/mongorestore_qa_test.go test/
git commit -m "migration: convert mongorestore core JS tests to Go"
```

---

### Task 10: Convert mongorestore oplog JS tests

**JS files:** `test/qa-tests/jstests/restore/oplog_*.js` and `preserve_oplog_structure_order.js` (9 files)
**Go target:** `mongorestore/oplog_test.go` (extend existing)

Audit results from the annotations (all NEW, all ReplSetTestType):
- `oplog_replay_and_limit.js` — NEW
- `oplog_replay_conflict.js` — NEW
- `oplog_replay_local_rs.js` — NEW
- `oplog_replay_noop.js` — NEW
- `oplog_replay_no_oplog.js` — NEW
- `oplog_replay_priority_oplog.js` — NEW
- `oplog_replay_size_safety.js` — NEW
- `oplog_replay_specify_file.js` — NEW
- `preserve_oplog_structure_order.js` — NEW

Read `mongorestore/oplog_test.go` in full before starting.

- [ ] **Step 1: Convert `oplog_replay_and_limit.js`** (NEW) — `TestOplogReplayWithLimit`: guard with `testtype.ReplSetTestType`. Create an oplog dump with many entries, restore with `--oplogLimit`, verify only entries before the limit are applied.

- [ ] **Step 2: Convert `oplog_replay_conflict.js`** (NEW) — `TestOplogReplayConflict`: guard with `testtype.ReplSetTestType`. Replay an oplog that inserts a document that already exists; verify conflict handling behavior.

- [ ] **Step 3: Convert `oplog_replay_local_rs.js`** (NEW) — `TestOplogReplayLocalRS`: guard with `testtype.ReplSetTestType`. Verify oplog replay works correctly against a replica set where the local DB is present.

- [ ] **Step 4: Convert `oplog_replay_noop.js`** (NEW) — `TestOplogReplayNoop`: guard with `testtype.ReplSetTestType`. Replay an oplog containing only no-op entries (type `n`), verify no errors and no data changes.

- [ ] **Step 5: Convert `oplog_replay_no_oplog.js`** (NEW) — `TestOplogReplayNoOplogFile`: guard with `testtype.ReplSetTestType`. Restore with `--oplogReplay` but no `oplog.bson` file present; verify appropriate error message.

- [ ] **Step 6: Convert `oplog_replay_priority_oplog.js`** (NEW) — `TestOplogReplayPriority`: guard with `testtype.ReplSetTestType`. Verify that `oplog.bson` at the root of the dump takes priority over per-DB oplog files.

- [ ] **Step 7: Convert `oplog_replay_size_safety.js`** (NEW) — `TestOplogReplaySizeSafety`: guard with `testtype.ReplSetTestType`. Attempt to replay an oplog entry exceeding the 16MB BSON document size limit; verify safe error handling.

- [ ] **Step 8: Convert `oplog_replay_specify_file.js`** (NEW) — `TestOplogReplaySpecifyFile`: guard with `testtype.ReplSetTestType`. Use `--oplogFile` to point to a specific oplog dump file rather than the default location.

- [ ] **Step 9: Convert `preserve_oplog_structure_order.js`** (NEW) — `TestRestorePreserveOplogOrder`: guard with `testtype.ReplSetTestType`. Verify that operations within the same transaction are applied in order.

- [ ] **Step 10: Run tests**

```bash
TOOLS_TESTING_TYPE=integration go test ./mongorestore/... -v -run ".*[Oo]plog.*" -count=1
```

- [ ] **Step 11: Commit**

```bash
git add mongorestore/oplog_test.go mongorestore/mongorestore_qa_test.go test/
git commit -m "migration: convert mongorestore oplog JS tests to Go"
```

---

### Task 11: Convert mongorestore users/roles + auth JS tests

**JS files:** `restore/users_and_roles*.js`, `restore/drop_authenticated_user.js`, `restore/nonempty_temp_users.js`, `restore/extended_json_metadata.js`, `restore/ordered_partial_index.js`, `restore/sharded_fullrestore.js`, `restore/write_concern_mongos.js`
**Go target:** `mongorestore/dumprestore_auth_test.go` (extend) + `mongorestore_qa_test.go`

Audit results from the annotations:
- `drop_authenticated_user.js` — EXTEND (`TestDumpRestorePreservesAdminUsersAndRoles` covers `--drop` but not verifying the restoring user itself survives the drop)
- `extended_json_metadata.js` — NEW (ShardedIntegrationTestType)
- `nonempty_temp_users.js` — EXTEND (`TestRestoreUsersOrRoles` covers cleanup but not the case where `admin.tempusers` already has data before restore begins)
- `ordered_partial_index.js` — NEW (ShardedIntegrationTestType)
- `sharded_fullrestore.js` — NEW (ShardedIntegrationTestType)
- `users_and_roles_admin.js` — EXTEND (`TestDumpRestorePreservesAdminUsersAndRoles` covers many scenarios but missing dump-without-flag and `--drop`-override cases)
- `users_and_roles.js` — EXTEND (`TestDumpRestoreSingleDBWithDBUsersAndRoles` covers main round-trip but `TestRestoreUsersOrRoles` only covers tempusers cleanup)
- `write_concern_mongos.js` — NEW (ShardedIntegrationTestType)

- [ ] **Step 1: Convert `users_and_roles.js`** (EXTEND) — In `dumprestore_auth_test.go`, add to `TestDumpRestoreSingleDBWithDBUsersAndRoles`: verify tempusers cleanup path fully (not just covered in `TestRestoreUsersOrRoles`).

- [ ] **Step 2: Convert `users_and_roles_admin.js`** (EXTEND) — In `dumprestore_auth_test.go`, add to `TestDumpRestorePreservesAdminUsersAndRoles`: the dump-without-`--dumpDbUsersAndRoles` case and the `--drop`-override case.

- [ ] **Step 3: Convert `nonempty_temp_users.js`** (EXTEND) — In `dumprestore_auth_test.go` or `mongorestore_qa_test.go`, add case: pre-populate `admin.tempusers` before starting restore; verify the restore correctly cleans up and replaces contents.

- [ ] **Step 4: Convert `drop_authenticated_user.js`** (EXTEND) — In `dumprestore_auth_test.go`, add case: restore with `--drop` when the collection contains the currently-authenticated user; verify the restoring user itself survives the drop.

- [ ] **Step 5: Convert `extended_json_metadata.js`** (NEW) — `TestRestoreExtendedJSONMetadata`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Create a `metadata.json` file using Extended JSON format for index keys and collection options; verify correct parsing during restore.

- [ ] **Step 6: Convert `ordered_partial_index.js`** (NEW) — `TestRestoreOrderedPartialIndex`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Dump/restore a collection with a partial index that has a sort key; verify correct restoration.

- [ ] **Step 7: Convert `sharded_fullrestore.js`** (NEW) — `TestRestoreShardedFullRestore`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Dump a sharded cluster, restore to a fresh sharded cluster (pointed at mongos), verify all collections and shard distributions are restored correctly.

- [ ] **Step 8: Convert `write_concern_mongos.js`** (NEW) — `TestRestoreWriteConcernMongos`: guard with `testtype.SkipUnlessTestType(t, testtype.ShardedIntegrationTestType)`. Restore with explicit write concern against a mongos; verify it succeeds.

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

Audit results:
- `ssl_with_system_ca.js` — NEW (no Go coverage; goes in new SSL integration test)
- `tls_with_system_ca.js` — NEW (no Go coverage; goes in new TLS integration test)

These tests verify that the tools can connect to MongoDB using the system's certificate authority store (not a custom CA file). This means connecting with `--tls` and without specifying `--tlsCAFile`.

- [ ] **Step 1: Create SSL integration test** — add `TestExportWithSystemCA` to a new file (e.g., `mongoexport/ssl_integration_test.go`) under `testtype.SkipUnlessTestType(t, testtype.SSLTestType)`. Connect without specifying a CA file; verify export completes.

- [ ] **Step 2: Create TLS integration test** — add `TestExportWithSystemTLSCA` similarly for the TLS variant.

- [ ] **Step 3: Document requirements** — the tests require `TOOLS_TESTING_MONGOD` to point to a TLS-enabled mongod with a system-trusted certificate. Add a comment in the test file explaining this.

- [ ] **Step 4: Run the test** manually in a suitable environment to verify; mark as needing manual verification in CI setup notes.

- [ ] **Step 5: Delete JS files, commit**

```bash
rm -r test/qa-tests/jstests/ssl/
rm -r test/qa-tests/jstests/tls/
git add mongoexport/ test/
git commit -m "migration: convert SSL/TLS JS tests to Go"
```

---

### Task 13: Audit and convert legacy42 JS tests

**JS files:** `test/legacy42/jstests/tool/` (34 files)

Audit results from the annotations:
- `command_line_quotes.js` — NEW (goes in `common/util` test or tool-specific test)
- `csv1.js` — EXTEND (csv_test.go covers unit-level CSV but no end-to-end CSV export then re-import with `--headerline`; goes in `mongoexport/mongoexport_test.go`)
- `csvexport1.js` — NEW (no Go test exports BSON types to CSV and verifies formatting)
- `csvexport2.js` — SKIP (csv_test.go covers CSV export variants)
- `csvimport1.js` — EXTEND (csv_test.go covers parsing but no Go test covers end-to-end import of multiline CSV with embedded quotes, empty strings, and leading/trailing whitespace; goes in `mongoimport/mongoimport_test.go`)
- `dumpauth.js` — EXTEND (`TestDumpRestoreEnforcesAuthRoles` covers backup/restore roles but not that system.profile is included in the dump; goes in `mongorestore/dumprestore_auth_test.go`)
- `dumprestore10.js` — NEW (ReplSetTestType; goes in `dumprestore_auth_test.go`)
- `dumprestore1.js` — EXTEND (basic dump/restore covered but missing `--collection` without `--db` error per SERVER-7721 and `--dir -` without `--db --collection` error; goes in `mongodump/mongodump_qa_test.go`)
- `dumprestore3.js` — NEW (ReplSetTestType; goes in `mongorestore_test.go`)
- `dumprestore4.js` — EXTEND (`TestMongorestore` covers nsFrom/nsTo rename and `TestCreateIndexes` covers index restoration, but no Go test restores to a renamed database and then verifies the index count on the renamed target; goes in `mongorestore/mongorestore_qa_test.go`)
- `dumprestore6.js` — EXTEND (`TestUnversionedIndexes` covers v:0 index restoration but `--keepIndexVersion` scenario is commented out per TOOLS-3020; verify `TestUnversionedIndexes` covers this test's assertions before deleting; goes in `mongorestore/mongorestore_qa_test.go`)
- `dumprestore7.js` — NEW (ReplSetTestType; goes in `mongorestore_test.go`)
- `dumprestore8.js` — NEW (no Go test creates capped collections, dumps, restores, and verifies capped behavior; goes in `mongorestore/mongorestore_qa_test.go`)
- `dumprestore9.js` — NEW (ShardedIntegrationTestType; goes in `mongorestore_test.go`)
- `dumprestore_excludecollections.js` — NEW (no Go test covers `--excludeCollection` and `--excludeCollectionsWithPrefix` for mongodump including error cases; goes in `mongodump/mongodump_qa_test.go`)
- `dumprestoreWithNoOptions.js` — EXTEND (Go tests use `--noOptionsRestore` only in timeseries context; no Go test verifies capped collections become non-capped across full/single-DB/single-collection restore; goes in `mongorestore/mongorestore_qa_test.go`)
- `dumpsecondary.js` — NEW (ReplSetTestType; goes in `mongodump_test.go`)
- `exportimport1.js` — NEW (no Go test covers export/import with undefined array elements and `--jsonArray` round-trip; goes in `mongoexport/mongoexport_test.go`)
- `exportimport3.js` — EXTEND (json_test.go covers format but no end-to-end `--jsonArray` export/import with 5 documents; goes in `mongoexport/mongoexport_test.go`)
- `exportimport4.js` — NEW (no Go test covers export/import with NaN values and query filters; goes in `mongoexport/mongoexport_test.go`)
- `exportimport5.js` — NEW (no Go test covers export/import with Infinity/-Infinity values and query filters; goes in `mongoexport/mongoexport_test.go`)
- `exportimport6.js` — NEW (no Go test covers `--sort`, `--skip`, `--limit` on export against real data; goes in `mongoexport/mongoexport_test.go`)
- `exportimport_bigarray.js` — NEW (no Go test covers `--jsonArray` export/import above 16MB BSON size limit; goes in `mongoexport/mongoexport_test.go`)
- `exportimport_minkey_maxkey.js` — NEW (no Go test covers MinKey/MaxKey as `_id` values surviving export/import round-trip; goes in `mongoexport/mongoexport_test.go`)
- `gridfs.js` — NEW (ShardedIntegrationTestType; goes in `mongofiles_test.go`)
- `restorewithauth.js` — SKIP (covered by `dumprestore_auth_test.go`)
- `shell_mkdir.js` — SKIP (shell mkdir utility, not relevant to Go migration)
- `stat1.js` — NEW (`mongostat_test.go` only has unit tests for StatLine; no Go integration test runs mongostat with auth; goes in `mongostat/mongostat_test.go`)
- `tool1.js` — EXTEND (basic dump/restore and export/import round-trips partially covered; this is a comprehensive smoke test; low priority; goes in `mongodump/mongodump_qa_test.go`)
- `tool_replset.js` — NEW (ReplSetTestType; goes in appropriate tool test file)
- `tsv1.js` — EXTEND (`tsv_test.go` covers unit-level TSV parsing but no Go test covers end-to-end TSV import with `-f fields` and `--headerline`; goes in `mongoimport/mongoimport_test.go`)

- [ ] **Step 1: Convert `csv1.js`** (EXTEND) — In `mongoexport/mongoexport_test.go`, add end-to-end case: export a collection to CSV, then re-import with `--headerline`, verify round-trip.

- [ ] **Step 2: Convert `csvexport1.js`** (NEW) — `TestExportCSVBSONTypes`: export ObjectId, BinData, ISODate, Timestamp, Regex, and function values to CSV; verify the formatting of each in the output.

- [ ] **Step 3: Convert `csvimport1.js`** (EXTEND) — In `mongoimport/mongoimport_test.go`, add end-to-end import case: multiline CSV with embedded quotes, empty strings, and leading/trailing whitespace.

- [ ] **Step 4: Convert `dumpauth.js`** (EXTEND) — In `mongorestore/dumprestore_auth_test.go`, add to `TestDumpRestoreEnforcesAuthRoles`: verify that `system.profile` is included in the dump output.

- [ ] **Step 5: Convert `dumprestore1.js`** (EXTEND) — In `mongodump/mongodump_qa_test.go`, add error cases: `--collection` without `--db` (per SERVER-7721) and `--dir -` without `--db --collection`.

- [ ] **Step 6: Convert `dumprestore4.js`** (EXTEND) — In `mongorestore/mongorestore_qa_test.go`, add case: restore to a renamed database (`--nsFrom`/`--nsTo`), then verify the index count on the renamed target is correct.

- [ ] **Step 7: Convert `dumprestore6.js`** (EXTEND) — In `mongorestore/mongorestore_qa_test.go`, verify `TestUnversionedIndexes` covers the v:0 index scenario from this test (the `--keepIndexVersion` part is commented out per TOOLS-3020); add any missing assertions.

- [ ] **Step 8: Convert `dumprestore8.js`** (NEW) — `TestRestoreCappedCollection`: create capped collections, dump them, restore, verify capped behavior (size limit, max documents) is preserved.

- [ ] **Step 9: Convert `dumprestore_excludecollections.js`** (NEW) — In `mongodump/mongodump_qa_test.go`, add `TestDumpExcludeCollectionsComprehensive`: test `--excludeCollection` and `--excludeCollectionsWithPrefix` including error cases.

- [ ] **Step 10: Convert `dumprestoreWithNoOptions.js`** (EXTEND) — In `mongorestore/mongorestore_qa_test.go`, extend `--noOptionsRestore` tests: verify capped collections become non-capped across full restore, single-DB restore, and single-collection restore.

- [ ] **Step 11: Convert `exportimport1.js`** (NEW) — `TestExportImportUndefinedArrayElements`: export/import with undefined array elements and `--jsonArray` round-trip.

- [ ] **Step 12: Convert `exportimport3.js`** (EXTEND) — Add end-to-end `--jsonArray` export/import with 5 documents; verify all documents survive the round-trip.

- [ ] **Step 13: Convert `exportimport4.js`** (NEW) — `TestExportImportNaN`: export/import with NaN values and query filters; verify NaN is handled correctly.

- [ ] **Step 14: Convert `exportimport5.js`** (NEW) — `TestExportImportInfinity`: export/import with Infinity and -Infinity values and query filters.

- [ ] **Step 15: Convert `exportimport6.js`** (NEW) — `TestExportSortSkipLimit`: cover `--sort`, `--skip`, `--limit` on export against real data. (This may overlap with Task 3 Step 15; combine if so.)

- [ ] **Step 16: Convert `exportimport_bigarray.js`** (NEW) — `TestExportImportBigArray`: `--jsonArray` export/import where the total size exceeds the 16MB BSON document size limit.

- [ ] **Step 17: Convert `exportimport_minkey_maxkey.js`** (NEW) — `TestExportImportMinKeyMaxKey`: verify MinKey and MaxKey as `_id` values survive export/import round-trip.

- [ ] **Step 18: Convert `stat1.js`** (NEW) — `TestMongoStatWithAuth`: run mongostat with correct and incorrect `--username`/`--password`; verify correct password succeeds and wrong password fails with auth error.

- [ ] **Step 19: Convert `tsv1.js`** (EXTEND) — In `mongoimport/mongoimport_test.go`, add end-to-end TSV import: import a TSV file with `-f` fields specification and with `--headerline`; verify document contents.

- [ ] **Step 20: Convert `tool1.js`** (EXTEND, low priority) — In `mongodump/mongodump_qa_test.go`, add a comprehensive smoke test covering basic dump/restore and export/import round-trips together.

- [ ] **Step 21: Convert `command_line_quotes.js`** (NEW) — Add to `common/util` or an appropriate tool test: verify command-line arguments with embedded quotes and spaces are handled correctly.

- [ ] **Step 22: Convert replica set tests** (NEW, all require `testtype.ReplSetTestType`):
  - `dumprestore10.js` → `TestDumpRestoreAuthReplSet` in `dumprestore_auth_test.go`
  - `dumprestore3.js` → `TestMongorestore_Legacy42_3` in `mongorestore_test.go`
  - `dumprestore7.js` → `TestMongorestore_Legacy42_7` in `mongorestore_test.go`
  - `dumpsecondary.js` → `TestDumpSecondary` in `mongodump_test.go`
  - `tool_replset.js` → `TestToolReplSet` in appropriate tool test file

- [ ] **Step 23: Convert sharded tests** (NEW, all require `testtype.ShardedIntegrationTestType`):
  - `dumprestore9.js` → `TestMongorestore_Legacy42_9` in `mongorestore_test.go`
  - `gridfs.js` → `TestGridFS_Sharded` in `mongofiles_test.go`

- [ ] **Step 24: Run the full integration test suite**

```bash
TOOLS_TESTING_TYPE=integration go test ./... -count=1
```

- [ ] **Step 25: Delete legacy42 directory, commit**

```bash
rm -r test/legacy42/
git add test/ mongodump/ mongorestore/ mongofiles/ mongoexport/ mongoimport/ mongostat/
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

| Batch | Task | JS files | Go target | Status | Notes |
|---|---|---|---|---|---|
| 0 | ShardedIntegrationTestType infra | — | `common/testtype/types.go`, `common.yml` | DONE (5b34c02b) | New test type + CI variant |
| 1 | Audit | All | — | DONE (53346f4c) | 21 SKIP, 111 NEW, 47 EXTEND |
| 2 | bsondump | 7 | `bsondump/bsondump_test.go` | | 2 SKIP, 3 NEW, 2 EXTEND |
| 3 | mongoexport | 19 | `mongoexport/mongoexport_test.go` | | 1 SKIP, 14 NEW, 4 EXTEND |
| 4 | mongoimport | 20 | `mongoimport/mongoimport_test.go` | | 2 SKIP, 11 NEW, 7 EXTEND |
| 5 | mongofiles | 16 | `mongofiles/mongofiles_test.go` | | 3 SKIP, 8 NEW, 5 EXTEND |
| 6 | mongostat | 7 | `mongostat/mongostat_test.go` | | 2 SKIP, 5 NEW |
| 7 | mongotop | 5 | `mongotop/mongotop_test.go` (new) | | 5 NEW (1 stress, skip) |
| 8 | mongodump | ~22 | `mongodump/mongodump_qa_test.go` (new) | | 5 SKIP, 12 NEW, 5 EXTEND |
| 9 | mongorestore core | ~38 | `mongorestore/mongorestore_qa_test.go` (new) | | 3 SKIP, 20 NEW, 15 EXTEND |
| 10 | mongorestore oplog | 9 | `mongorestore/oplog_test.go` | | 9 NEW (all ReplSetTestType) |
| 11 | mongorestore auth/sharded | 8 | `mongorestore/dumprestore_auth_test.go` + `mongorestore_qa_test.go` | | 4 NEW (sharded), 4 EXTEND |
| 12 | SSL/TLS | 2 | new SSL/TLS integration test files | | 2 NEW; manual env setup required |
| 13 | legacy42 | 34 | Various | | 4 SKIP, 20 NEW, 10 EXTEND |
| 14 | Cleanup | — | `common.yml`, `build.go`, `scripts/` | | Remove infrastructure |

**Consistent SKIPs across all batches:**
- Broken pipe tests — OS-signal behavior, untestable in Go unit/integration tests
- Tests disabled in JS with comments — respect the existing disable rationale
