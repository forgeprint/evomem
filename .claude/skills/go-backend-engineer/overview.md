# Go Senior Backend Engineer

## What it changes

Go makes a working HTTP service short to write, and the short version has no
timeouts. `http.Server{}` waits for headers forever, `http.DefaultClient` waits
for responses forever, `sql.Open` allows unlimited connections, and a
goroutine started from a handler has nobody who stops it. None of these fails a
test. An agent without this expert writes all of them. With it:

- **A context reaches every call that can block.** It is the first parameter,
  `Background()` appears only in `main` and tests, and `NewRequestWithContext`
  and `...Context` database calls are used throughout. In Gin, the request's
  context goes downstream and a goroutine gets `c.Copy()`.
- **Every zero-valued timeout is replaced.** The server gets all four timeouts
  (gosec G112 and G114 enforce this), one shared client gets a `Timeout`,
  bodies are capped with `MaxBytesReader`, and unknown JSON fields are
  rejected.
- **Errors wrap with `%w` and are matched with `errors.Is`.** One function maps
  them to RFC 9457. slog writes JSON with the request's trace id and a
  `LogValuer` that redacts secrets.
- **The pool is sized and everything borrowed is returned.** Rows are closed
  and `Err()` is checked, and `defer tx.Rollback()` follows every `BeginTx`.
- **Every goroutine has an owner and an end**, checked by goleak in tests and
  by Go 1.27's `goroutineleak` profile in production.

Six checklists and nine refusals, most of them a vet analyzer, a
golangci-lint linter or a gosec rule.

## What it fits

- HTTP services on Go 1.27 using `net/http` (with the 1.22+ routing patterns)
  or Gin, with PostgreSQL through `database/sql` or pgx.
- Reviewing a Go service that slows down or leaks memory under load. The
  timeout, pool and goroutine checklists cover those failures.
- Hardening a service before it faces untrusted traffic.

## What it does not fit

- **Architecture.** Package layout, service boundaries and the persistence
  model belong to the architect. The catalog's only architect today is
  [`dotnet-senior-architect`](../dotnet-senior-architect/SKILL.md); Go
  architects are planned.
- **The API contract**, which belongs to the API designer. This expert only
  keeps the OpenAPI document next to the code and checked.
- **A security audit.** That goes to [`security-reviewer`](../security-reviewer/SKILL.md).
- **Schema and query design.** That goes to [`sql-data-engineer`](../sql-data-engineer/SKILL.md).
- **CLIs, libraries and gRPC-only services.** The timeout and shutdown rules
  assume an HTTP server, and a library has no `main`.
- **Echo, Fiber, chi and other routers.** The `net/http` rules still apply to
  them. The Gin-specific rows do not.

## Pros and cons

**In its favour:** nearly every row maps to a `go vet` analyzer, a
golangci-lint linter, a gosec rule ID or a test that can fail. The expert takes
effect in `.golangci.yml` and CI, and stays in effect after the agent is
finished. It names the zero values that cause trouble, and each one takes a
single line to fix.

**Against it:** it asks for a long linter list. Some of those linters, such as
`wrapcheck`, are noisy in a codebase that was not written with them on. It
assumes PostgreSQL and Kubernetes-style probes. It was drafted by a tool from
documentation and source, not from a person's review history, and
`provenance: generated` records that.
