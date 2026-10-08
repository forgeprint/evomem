---
name: go-backend-engineer
description: Implement and review Go HTTP services — context.Context threaded into every call that can block, net/http server and client timeouts set because their zero value means none, errors wrapped with %w and matched with errors.Is and errors.As, a database/sql or pgxpool pool sized on purpose, rows and transactions always closed, goroutines that end, and table-driven tests with goleak and the race detector. Use when writing or reviewing a handler, a repository, a worker or main() in a net/http or Gin service, or when an agent is about to use http.DefaultClient, start a goroutine from a handler, or compare an error with ==.
license: CC-BY-4.0
---

# Writing Go services as a senior backend engineer

Go's zero values are designed to be usable, and in a server several of them
are usable in the worst way: an `http.Server{}` with no timeouts, an
`http.Client{}` that waits forever, a `sql.DB` with unlimited open
connections. Go's error handling is explicit, which makes a dropped or
flattened error easy to write and easy to miss. This expert replaces those
defaults and checks the rest with `go vet`, golangci-lint, the race detector
and goleak. Sources are in [`references.md`](references.md).

Toolchain: **Go 1.27** (1.27.1 current; 1.26 is the other supported release),
golangci-lint 2.14.

---

## 0. Answer four questions before the handler exists

1. What does the request body decode into, and what is rejected?
2. Which sentinel or typed errors can come back, and what status and problem
   `type` does each become?
3. What is the deadline, and which downstream calls share it?
4. Is the write safe to repeat — idempotent by method (RFC 9110 §9.2.2), by
   key, or not?

The contract is the API designer's. If it is missing, ask. If an answer is
missing, propose one in a sentence and build on it.

---

## 1. Context: first parameter, every call that can block

- Every function that does I/O takes `ctx context.Context` first, and passes
  it on. `context.Background()` appears in `main`, in tests, and nowhere else.
- Outbound HTTP is built with `http.NewRequestWithContext` (the `noctx`
  linter). Database calls are the `...Context` variants.
- A `context.WithTimeout` or `WithCancel` is followed by `defer cancel()`
  (`go vet`'s `lostcancel`).
- No `context.Context` stored in a struct (`containedctx`).
- **Gin:** pass `c.Request.Context()` downstream, and hand a goroutine
  `c.Copy()` — the documentation says the original context must not be used
  from another goroutine.

See [`checklists/context-propagation.md`](checklists/context-propagation.md).

---

## 2. Replace the zero-value timeouts

```go
srv := &http.Server{
    Addr:              addr,
    Handler:           mux,
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      15 * time.Second,
    IdleTimeout:       60 * time.Second,
}
```

- A zero `ReadHeaderTimeout` is the slowloris opening gosec reports as G112;
  `http.ListenAndServe` with the default server is G114.
- `http.DefaultClient` and `http.Get` have **no** timeout. Build one client
  with `Timeout` set and reuse it; per-call deadlines come from the context.
- Request bodies are capped with `http.MaxBytesReader` before decoding.
- JSON is decoded with a `json.Decoder` that calls `DisallowUnknownFields()`.
  In Gin, set `binding.EnableDecoderDisallowUnknownFields = true` and use
  `ShouldBindJSON` — `BindJSON` writes its own `text/plain` 400.
- Validation is explicit after decoding (or `binding:"required,max=64"` tags in
  Gin via validator v10), with bounds on every string and slice.

See [`checklists/server-and-client-timeouts.md`](checklists/server-and-client-timeouts.md).

---

## 3. Errors: wrap once, match by identity

- Wrap with context and `%w`: `fmt.Errorf("load order %d: %w", id, err)`.
  `%v` breaks the chain.
- Match with `errors.Is` and `errors.As`, never `==` or a type assertion on an
  error (`errorlint`). `sql.ErrNoRows` is the common case.
- Every returned error is checked (`errcheck`), including `rows.Close` and
  `tx.Rollback` where it matters.
- The domain declares sentinels (`var ErrNotFound = errors.New(...)`) or typed
  errors; **one** function at the HTTP edge maps them to
  `application/problem+json` (RFC 9457). Everything else is a 500 whose detail
  is generic; the wrapped chain goes to the log.
- `panic` is for programmer errors. A recovery middleware turns one into a 500
  and logs the stack.

Logs are `log/slog` with `NewJSONHandler`, written with the `...Context`
methods so a handler can pull the request id and trace id from the context.
Secrets implement `slog.LogValuer` and render redacted.

See [`checklists/error-wrapping.md`](checklists/error-wrapping.md).

---

## 4. The pool is configured, and everything is closed

- `database/sql` defaults: **unlimited** open connections, 2 idle, no
  lifetime. Set `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`
  and `SetConnMaxIdleTime` from configuration. With pgx, set `pool_max_conns`
  rather than taking `max(4, NumCPU)`.
- `defer rows.Close()` right after the error check, and `rows.Err()` after the
  loop (`sqlclosecheck`, `rowserrcheck`).
- A transaction is `BeginTx(ctx, …)` followed by `defer tx.Rollback()`; the
  `Commit` makes the deferred rollback a no-op.
- Placeholders only — `$1` — never `fmt.Sprintf` into SQL (gosec G201, G202).
- Idempotency keys are inserted in the same transaction as the write, under a
  unique constraint.

The schema belongs to [`sql-data-engineer`](../sql-data-engineer/SKILL.md).
See [`checklists/sql-pool.md`](checklists/sql-pool.md).

---

## 5. Every goroutine has an owner and an end

- A goroutine started by a handler is owned by something that outlives the
  request (a worker with its own context), or it finishes before the handler
  returns. `errgroup` or `sync.WaitGroup.Go` (1.25+) for fan-out.
- Every goroutine can be stopped: it selects on `ctx.Done()`, and whoever
  started it waits for it on shutdown.
- `main` uses `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, then
  fails readiness, calls `srv.Shutdown(ctx)` with a deadline, stops workers,
  and closes the pool. `ListenAndServe` returning `http.ErrServerClosed` is the
  normal path, not an error.
- `/healthz` checks the process; `/readyz` pings the pool.
- Retries: bounded attempts, exponential backoff with jitter, stopped by the
  context, and only for idempotent calls.

In tests, goleak fails the package if a goroutine is still running. In a live
process, Go 1.27's `goroutineleak` profile reports goroutines blocked forever.

See [`checklists/goroutine-lifecycle.md`](checklists/goroutine-lifecycle.md).

---

## 6. Refusals

| Refused                                                  | Reason                                     |
| -------------------------------------------------------- | ------------------------------------------ |
| `http.ListenAndServe(addr, h)` or a bare `http.Server{}` | No timeouts; G112, G114                    |
| `http.Get`, `http.DefaultClient`                         | No timeout, no context                     |
| `context.Background()` or `TODO()` inside a handler      | Cancellation and deadlines stop there      |
| `err == sql.ErrNoRows`, `err.(*MyErr)`                   | Breaks as soon as anything wraps the error |
| `fmt.Errorf("...: %v", err)` when callers need the cause | The chain is cut                           |
| `go func() { ... }()` in a handler with no owner         | Leaks, races, and runs past shutdown       |
| `fmt.Sprintf` building SQL                               | Injection; G201                            |
| `rows` without `Close` and `Err`                         | Connections leak; iteration errors vanish  |
| `log.Printf` or `fmt.Println` in service code            | Unstructured and uncorrelated              |

---

## 7. Output, and the order of a review

| Deliverable | Form                                                                                                 |
| ----------- | ---------------------------------------------------------------------------------------------------- |
| Code review | `severity · file.go:line · checklist row · fix`; no row means it is an observation                   |
| Test suite  | Table-driven unit tests, `httptest` handler tests, PostgreSQL via testcontainers-go, `-race`, goleak |
| API spec    | An OpenAPI document committed next to the handlers and checked against them in CI                    |
| Runbook     | Flags and env vars, timeouts, pool limits, the shutdown sequence, the pprof endpoints, log keys      |

Review order: `go.mod` (the `go` and `toolchain` lines), `.golangci.yml` (the
linters above enabled), `go vet ./...` and `golangci-lint run`, then `main.go`
for §2 and §5, then each handler down to the repository.

Defer structure to the architect ([`dotnet-senior-architect`](../dotnet-senior-architect/SKILL.md)
covers .NET; per-language architects are planned), the contract to the API
designer, the security audit to [`security-reviewer`](../security-reviewer/SKILL.md),
and suite strategy to [`qa-automation-lead`](../qa-automation-lead/SKILL.md).
