# ADR-0011: pgx is the second dependency

**Status:** accepted · **Date:** 2026-10-06

## Context

Phase 5 syncs to PostgreSQL. ADR-0002 allows the standard library and
`modernc.org/sqlite`, and nothing else without a decision of its own.

`database/sql` is in the standard library; a PostgreSQL driver is not. The
options were:

1. **`jackc/pgx/v5`** in its `stdlib` mode. Pure Go, the de facto driver in
   the Go ecosystem, actively maintained.
2. **`lib/pq`.** Pure Go, one module, smaller — but upstream is in
   maintenance mode and no longer tracks new PostgreSQL features.
3. **Speak to PostgreSQL over HTTP** through something like PostgREST. No new
   Go dependency, but it trades a library for a service that has to be run and
   secured, and writes through an API layer rather than to the database.
4. **Write the PostgreSQL wire protocol by hand.** Not proportionate.

The user chose 1.

## Decision

Depend on `github.com/jackc/pgx/v5`, used through `database/sql` with the
`stdlib` driver, and vendor it. The import path and the registered driver name
`"pgx"` were checked against pkg.go.dev on 2026-10-06.

## Consequences

- Two direct dependencies now, and six modules in the tree rather than one:
  `pgx/v5`, `pgpassfile`, `pgservicefile`, `puddle/v2`, `golang.org/x/sync`,
  `golang.org/x/text`. `vendor/` grew from 136 MB to 144 MB.
- Still no cgo. `scripts/crosscheck.sh` compiles all five targets with
  `CGO_ENABLED=0`, so that is checked rather than assumed.
- `core/sync` is written against its own `Remote` interface and `Postgres` is
  one implementation of it. The driver is reachable from one file; a second
  transport, or a replacement, does not touch the worker.
- The SQL is tested against a real PostgreSQL 17, not asserted. The tests are
  skipped unless `EVOMEM_TEST_POSTGRES_DSN` is set, so `go test ./...` still
  needs no database:

  ```sh
  docker run -d --rm --name evomem-pg -e POSTGRES_PASSWORD=test \
    -e POSTGRES_DB=evomem_test -p 55432:5432 postgres:17-alpine

  EVOMEM_TEST_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55432/evomem_test?sslmode=disable' \
    go test ./core/sync/
  ```

- `scripts/ci.sh` does not run them, because it has to stay runnable offline
  on any machine. They are a separate step, like Forgelore's drift checks.
