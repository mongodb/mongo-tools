# mongodump task generator

This program generates the checked-in Evergreen configuration for the mongodump/mongorestore
passthrough tasks and their build variants. It writes
[`evergreen/config/mongodump-passthrough.yml`](../../evergreen/config/mongodump-passthrough.yml),
which is included by `common.yml`.

It was ported from `cmd/mongodump-task-gen` in the mongosync repo. Unlike the mongosync copy, it
does not generate tasks at runtime via `generate.tasks`; the config is generated once and checked
in, and CI verifies it is up to date (the `mongodump_config_up_to_date` task).

## Usage

Run it from the root of the mongo-tools checkout:

```
go run ./dev-tools/mongodump-task-gen
```

This regenerates `evergreen/config/mongodump-passthrough.yml` and formats it with `precious`. Commit
the result. To run the passthrough tests on a patch, select one or more of the generated
`mongodump-<src>-to-<dst>-amazon2-arm64` variants.

## Layout

- `generate/` — the generator; `Generator.Configuration` builds the build variants and tasks and
  `Generator.WriteConfig` writes them to the config file
- `versions/` — server version parsing and the pinned resmoke/server versions, ported from
  mongosync's `mongo-go/versions`
