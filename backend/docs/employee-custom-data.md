# Employee Custom Data

Agencies often need to attach fields to their officers that this codebase doesn't model as
columns — badge numbers, internal HR IDs, and other agency-specific attributes. `users` has a
`custom_data JSONB` column for exactly this, optionally validated against a JSON Schema you
configure per deployment.

This is **single-agency-per-deployment**: this codebase has no `agencies`/`employees` table —
every deployment is one agency's own database — so "per-agency schema" means **one schema per
deployment**, set once and applying to every employee in it, not a per-employee or multi-tenant
schema registry.

## Storage

Added in [`migrations/000007_add_users_custom_data.sql`](../migrations/000007_add_users_custom_data.sql):
a real Postgres `jsonb` column (not this codebase's usual SQLite-portable TEXT+JSONB-type
convention) plus a Postgres-only GIN index, authored using the dialect-scoped `-- @postgres`
sub-blocks described in [`migrations.md`](./migrations.md). The GIN index isn't used by anything
yet — it exists to support containment-query filtering later (see [Future work](#future-work)).

The Go side reuses the existing `dbtype.JSONB` type (`pkg/dbtype`, originally
`internal/application`'s JSONB column type) for `UserRecord.CustomData`
(`internal/user/store.go`), and validation reuses the shared
`jsonschemautil.ValidateInstance` helper (`pkg/jsonschemautil`) also used by
`internal/application` and `internal/certificate` — one schema-validation code path for all three.

## Configuring the schema

Set `USER_CUSTOM_DATA_SCHEMA_PATH` to a JSON Schema file (see [`.env.example`](../.env.example)):

```
USER_CUSTOM_DATA_SCHEMA_PATH=./config/user-custom-data-schema.json
```

Left unset, `customData` is stored as-is with no validation. The schema is read once at CLI
startup — it's only wired into `nswac` (`cmd/cli`), since `cmd/server` has no code path that
writes `CustomData` (see [Limitations](#limitations) below).

### Example

```json
{
  "type": "object",
  "required": ["badgeNumber"],
  "properties": {
    "badgeNumber": { "type": "string", "pattern": "^[0-9]{5}$" },
    "department": { "type": "string" },
    "internalId": { "type": "string" }
  },
  "additionalProperties": false
}
```

## Setting custom data

`customData` is only ever set via `nswac user add`, and only at **first creation** — re-seeding
an existing user does not reapply or re-validate it. See `nswac user add -h` for the full CLI
usage.

File-based (`--file`):

```json
{
  "users": [
    {
      "name": "Jane Doe",
      "email": "jane@agency.gov.au",
      "roles": ["lab_officer"],
      "customData": { "badgeNumber": "12345" }
    }
  ]
}
```

Interactive `nswac user add` (no `--file`) also prompts for an optional custom data JSON object.

If a schema is configured, every user in the batch is validated before any of them are inserted —
one invalid `customData` fails the whole import, so a partially-invalid seed file never partially
applies.

## Evolving the schema

The schema is deployment config, not versioned or migrated — changing it is a config change plus
a service restart, not a database migration. Adding a field (e.g. `location`):

1. Edit the schema file `USER_CUSTOM_DATA_SCHEMA_PATH` points to and restart the service —
   `custom_data` is already a `jsonb` column, so no schema migration is needed for the new field
   itself.
2. Existing rows are **not** revalidated or backfilled. Employees seeded before the change keep
   whatever `customData` they already had, which may now be missing the new field.
3. If you need every existing employee to have the field, backfill it yourself (e.g. a one-off
   `UPDATE users SET custom_data = custom_data || '{"location": "..."}'` per row, or a re-seed —
   note re-seeding does **not** touch `customData` on an existing user, so backfilling still needs
   a direct update).

Because of step 2, prefer making new fields optional (or give the deployer a migration plan for
existing rows) rather than `required` — a `required` addition only affects employees created
*after* the change, silently leaving older rows non-conforming to the schema you're now shipping.

## Limitations

- **No live write endpoint.** `customData` is set only via the seed CLI; there's no
  `PATCH /api/v1/users/{userId}/custom-data` today. `rbacMiddleware.RequireAction` — the natural
  fit for such an endpoint — resolves permissions via a `taskId` path param and a task config
  file, with no equivalent concept for a user/employee resource; a generic "admin can edit any
  user" authorization check would be new plumbing this codebase doesn't have yet.
- **Not surfaced on `GET /api/v1/users/me`.** `ProfileService.GetMe` doesn't return `CustomData`.
  `UserStore.GetCustomData`, a lookup by `UserID`, now exists (added for
  [data-scoping.md](./data-scoping.md)'s internal use), so this is now just a `GetMe` wiring gap,
  not a missing store capability.

That's a natural follow-up once a concrete read need shows up at the API layer.

## Future work

With a real `jsonb` column, filtering becomes `users.custom_data @> '{"field": "value"}'`
(containment), joined against `applications.claimed_by` (the existing officer-attribution column)
to filter consignments/applications by employee custom-data fields — e.g. "which consignments
were handled by the employee with internal ID X". Not built yet; likely a new query param on
`GET /api/v1/applications` or `/api/v1/consignments` once the concrete field(s) to filter by are
known.

This is different from [data-scoping.md](./data-scoping.md), which is now built: that compares the
*requesting* officer's own `custom_data` against a consignment's, not the claimant's.
