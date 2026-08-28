# Migrations

Schema changes are plain `.sql` files in [`migrations/`](../migrations), applied with
`go run ./cmd/migrate up` (the CLI also has `down`, `status`, and `generate <name>` to scaffold a
new file). They are parsed and applied by [`internal/migrator`](../internal/migrator) — nothing
is generated or inferred from Go structs; the `.sql` file is the single source of truth for the
schema.

## File Structure

Each file is named `<version>_<name>.sql` (e.g. `000007_add_users_custom_data.sql`) and contains
an `-- @UP` block and an optional `-- @DOWN` block:

```sql
-- @UP
ALTER TABLE users ADD COLUMN custom_data TEXT;

-- @DOWN
ALTER TABLE users DROP COLUMN custom_data;
```

`@UP` needs at least one statement; `@DOWN` is optional — a migration can be irreversible.
Versions must be contiguous (no gaps) and unique; `go run ./cmd/migrate generate <name>` picks
the next version number for you and scaffolds the file.

## Driver-Specific SQL

This app runs on both SQLite (local dev/test default) and Postgres (production). Most migrations
are plain, portable SQL that runs unchanged on both. When a statement is genuinely
driver-specific — a Postgres `USING GIN` index, a `jsonb`-only operator, a SQLite `PRAGMA` — nest
a `-- @postgres` or `-- @sqlite` marker inside the `@UP`/`@DOWN` block it belongs to:

```sql
-- @UP
ALTER TABLE users ADD COLUMN custom_data JSONB;

-- @postgres
CREATE INDEX IF NOT EXISTS idx_users_custom_data ON users USING GIN (custom_data);

-- @DOWN
ALTER TABLE users DROP COLUMN custom_data;

-- @postgres
DROP INDEX IF EXISTS idx_users_custom_data;
```

Rules:
- A dialect marker scopes every following line to that driver until the next marker (another
  dialect marker, or a new `@UP`/`@DOWN`) is encountered. Entering a new section always resets
  the scope back to portable/generic — a dialect scope never carries across `@UP`/`@DOWN`.
- On `up`, the portable block runs first, then the current driver's block (if any). On `down`,
  the order is reversed: the driver-specific block runs first, then the portable block — so an
  addition made in a driver-specific `@UP` block (like an index) is undone before the portable
  change it depends on. Write teardown SQL idempotently (`DROP INDEX IF EXISTS`, `IF NOT EXISTS`
  on creation) so it stays safe if a cascade already handled it.
- A migration can be entirely driver-specific (e.g. only a `-- @postgres` sub-block, no portable
  statement) — the "at least one statement" requirement on `@UP` counts dialect sub-blocks as
  content.
- An unrecognized `--@...` annotation (a typo like `-- @postgress`, or any other `--@` line) is a
  parse error rather than being silently treated as an inert SQL comment — this reserves the
  `--@` prefix for migration annotations. A normal SQL comment that doesn't start with `--@` is
  unaffected.
- Every migration written before this feature has no dialect markers and keeps behaving exactly
  as before; dialect blocks are strictly additive.