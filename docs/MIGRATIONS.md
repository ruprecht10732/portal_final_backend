# Database Migrations & Schema Management

## Overview

This project uses two complementary systems:

| System | Purpose | Location |
|--------|---------|----------|
| **Runtime migrations** | Apply DDL changes on deploy/startup | `migrations/` (001–083+) |
| **sqlc schema input** | Generate type-safe Go query code | `migrations/` |

These are **separate concerns**. Runtime migrations evolve the live database.
sqlc reads the same migration files to infer the current table shapes so
`sqlc generate` can produce correct Go structs and query helpers.

---

## Runtime Migration Runner

**Entry point:** `platform/db/migrate.go` → `RunMigrations()`

Called at startup from `cmd/api/main.go` with up to 5 retries.

### How it works

1. Connects to PostgreSQL.
2. Uses **goose** to track versions in `goose_db_version`.
3. **Bootstrap check** (one-time): if the goose table is missing *and* the DB
   already has application tables (`rac_users`, `rac_leads`, etc.), goose is
   seeded to the latest migration number so history is not re-applied.
4. Runs pending migrations from `migrations/` in order.

### Key properties

- **Idempotent** — re-running skips already-applied versions.
- **Transactional** — goose executes migrations inside a transaction when possible.
- **Bootstraps existing DBs** without reapplying historical files.

---

## Creating a New Migration

1. **Add a file** in `migrations/` with the next sequence number:

   ```
   migrations/080_describe_change.sql
   ```

2. **Use goose markers** (required for SQL migrations):

   ```sql
   -- +goose Up
   ALTER TABLE rac_leads ADD COLUMN foo TEXT;

   -- +goose Down
   ALTER TABLE rac_leads DROP COLUMN IF EXISTS foo;
   ```

   Only the Up section runs automatically. Down is for rollback tooling.

3. **Regenerate Go code:**

   ```bash
   sqlc generate
   ```

4. **Update queries** (if needed) in `internal/*/sql/queries.sql` to use the
   new column.

5. **Verify:**

   ```bash
   go build ./...
   go test ./...
   ```

---

## sqlc Schema Input

sqlc reads `migrations/` directly. There are no separate schema files.

- Migrations are the **single source of truth** for runtime DDL and sqlc codegen.
- If a migration adds/changes a column, sqlc sees it automatically on
  `sqlc generate`.

---

## Provisioning a Fresh Database

For a brand-new environment:

1. **Option A — Run all migrations** (recommended):

   ```bash
   # The app does this automatically on startup.
   # migrations/ 001–083 recreate the full schema from scratch.
   ```

2. **Option B — Restore from baseline** (faster for dev/CI):

   ```bash
   psql "$DATABASE_URL" < docs/baseline_schema.sql
   ```

   Then start the app; the bootstrap logic will seed `goose_db_version` and
   apply any new files (080+).

`docs/baseline_schema.sql` is a `pg_dump --schema-only` snapshot of the
production database taken at the 083 migration mark.

---

## Queries

| Domain | Query file | Notes |
|--------|-----------|-------|
| Auth | `internal/auth/sql/queries.sql` | 17+ named queries (CRUD users, tokens, roles) |
| Catalog | `internal/catalog/sql/queries.sql` | Full CRUD for VAT rates, products, materials |
| Leads | `internal/leads/sql/queries.sql` | Starter queries — expand incrementally |

### Writing queries

```sql
-- name: GetLeadByID :one
SELECT * FROM rac_leads WHERE id = $1;

-- name: ListLeadsByOrg :many
SELECT * FROM rac_leads
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
```

After adding queries, run `sqlc generate` and verify with `go build ./...`.

---

## Quick Reference

```bash
# Regenerate Go code from schemas + queries
sqlc generate

# Build & verify
go build ./...

# Run tests
go test ./...

# Dump current live schema (for updating baseline)
pg_dump --schema-only --no-owner --no-privileges \
  --exclude-table='goose_db_version' \
  "$DATABASE_URL" > docs/baseline_schema.sql
```

---

## Directory Layout

```
portal_final_backend/
├── migrations/                        # Runtime DDL (001–079+)
│   ├── 001_init.sql
│   ├── …
│   └── 083_google_lead_webhooks.sql
├── docs/
│   ├── baseline_schema.sql            # pg_dump snapshot at migration 079
│   └── MIGRATIONS.md                  # ← this file
├── sqlc.yaml                          # sqlc codegen config
├── internal/
│   ├── auth/
│   │   ├── sql/queries.sql            # sqlc queries
│   │   └── db/                        # generated Go (authdb)
│   ├── leads/
│   │   ├── sql/queries.sql            # sqlc queries
│   │   └── db/                        # generated Go (leadsdb)
│   └── catalog/
│       ├── sql/queries.sql            # sqlc queries
│       └── db/                        # generated Go (catalogdb)
└── platform/
    └── db/
        └── migrate.go                 # Migration runner
```
