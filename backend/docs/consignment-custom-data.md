# Consignment Custom Data

Restricting the consignment list view by an officer's own location needs a location value on the
consignment itself — but the data that carries it arrives per-application (per task), not
per-consignment, and different tasks under the same consignment can carry different, differently
shaped payloads. `consignments` has a `custom_data JSONB` column for this: each task config can
declare rules that copy specific fields out of its injected application data and onto the parent
consignment, accumulating across every task that touches it over its lifetime.

This document covers only the plumbing — getting fields onto `consignments.custom_data`. The
actual location-based view filter (joining it against the requesting officer's own
`users.custom_data`, see [employee-custom-data.md](./employee-custom-data.md)) is
[data-scoping.md](./data-scoping.md).

Note: `consignments` already has a different JSONB column, `nsw_data` — that's unrelated display
metadata (e.g. trader company name) fetched once from NSW Core and cached on first sight of the
consignment. `custom_data` is agency-derived and accumulated instead. Don't confuse the two.

## Storage

Added in [`migrations/000009_add_consignments_custom_data.sql`](../migrations/000009_add_consignments_custom_data.sql):
a real Postgres `jsonb` column, same convention as [`users.custom_data`](./employee-custom-data.md).
Unlike that migration, this one has **no GIN index** — `consignments` is a live, high(er)-write,
request-path table (touched on every application inject/review/feedback/claim), not a small
CLI-seed-only one, so bundling a blocking `CREATE INDEX` the way `users.custom_data` did isn't a
safe default here. The index is a deliberate follow-up once there's visibility into actual row
counts — see [Limitations](#limitations).

## Pushing fields from a task

A task config declares which fields to copy from its injected application `data` onto the
consignment, via `consignmentFields`:

```json
{
  "consignmentFields": [
    { "source": "/importer/address/district", "target": "/district" },
    { "source": "/logistics/portOfEntry", "target": "/portOfEntry" }
  ]
}
```

`source` and `target` are both [JSON Pointers](https://datatracker.ietf.org/doc/html/rfc6901)
(`/a/b/c`), resolved and written against `map[string]any` objects only — arrays are deliberately
unsupported (see [Limitations](#limitations)). A `source` that doesn't resolve (missing, or the
path passes through an array) is silently skipped, not an error — most injected payloads won't
carry every field every task might care about. A malformed `source`/`target` (missing the leading
`/`) is rejected at task-config load time, same as the rest of `TaskConfig.Validate()`.

Every application inject re-evaluates its task's `consignmentFields` against the latest injected
data and merges the result onto the consignment — including on a trader's resubmission after
feedback, so a corrected value replaces the old one. Fields accumulate across different tasks: if
task A pushes `district` and task B (injected separately, maybe weeks later) pushes `portOfEntry`,
the consignment ends up with both. On a repeated key, whichever push happened most recently wins;
nothing detects or prevents two task configs targeting the same field.

## Configuring the schema

Set `CONSIGNMENT_CUSTOM_DATA_SCHEMA_PATH` to a JSON Schema file (see
[`.env.example`](../.env.example)):

```
CONSIGNMENT_CUSTOM_DATA_SCHEMA_PATH=./config/consignment-custom-data-schema.json
```

Left unset, pushed fields are stored as-is with no validation. Unlike
[`users.custom_data`](./employee-custom-data.md), this schema is read once at **server** startup,
not by the CLI — the only writer of `consignments.custom_data` is the running server's inject flow.

The schema validates the *whole merged document* after every push, not just the fields from one
task — so avoid `required` on these properties: a consignment only has whatever fields the tasks
that have touched it so far have pushed, and a `required` field would reject every consignment
until every contributing task has run. Use it for shape/type safety instead:

```json
{
  "type": "object",
  "properties": {
    "district": { "type": "string" },
    "portOfEntry": { "type": "string" }
  },
  "additionalProperties": false
}
```

If a push would leave the merged document failing the schema, the merge is skipped (logged as a
warning) — the application inject itself still succeeds. A downstream enrichment feature must
never be able to fail the primary inject flow.

## Evolving the schema

The schema is deployment config, not versioned or migrated — changing it is a config change plus a
service restart, not a database migration. Adding a field (e.g. `vesselName`):

1. Edit the schema file `CONSIGNMENT_CUSTOM_DATA_SCHEMA_PATH` points to, add a `consignmentFields`
   rule to whichever task config(s) should push it, and restart the service — `custom_data` is
   already a `jsonb` column, so no schema migration is needed for the new field itself.
2. Existing consignments are **not** revalidated or backfilled. A consignment whose contributing
   tasks were injected before the change keeps whatever `custom_data` it already had, which may
   now be missing the new field — and since fields only get pushed on an inject (create, re-inject,
   or resubmission), an already-`DONE` consignment may never get the new field at all.
3. If you need existing consignments to have the field, backfill it yourself (e.g. a one-off
   `UPDATE consignments SET custom_data = custom_data || '{"vesselName": "..."}'` per row).

Because of step 2, prefer optional properties over `required` even more than for
[`users.custom_data`](./employee-custom-data.md) — a consignment's fields depend on which tasks
have actually run against it, not just when it was created.

## Limitations

- **No array support in `source`/`target`.** The resolver only walks `map[string]any`; the moment
  it meets an array anywhere in the path, that rule just doesn't resolve. This is deliberate, not
  a gap to close casually — see the design note in [`pkg/jsonpointer`](../pkg/jsonpointer). JSON
  Pointer syntax already supports array indices, so this can be extended later without a task
  config format change, if a genuine need for it shows up.
- **No GIN index yet** on `consignments.custom_data` — see [Storage](#storage).
- **No cross-task conflict detection.** Two task configs pushing to the same `target` silently
  last-write-wins; nothing warns you at config-authoring time.

## Future work

See [data-scoping.md](./data-scoping.md) for the location-based (and generally attribute-based)
view filter this column exists to support — implemented as its own `internal/datascope` package.
The GIN index above is still outstanding, now that filter is live.
