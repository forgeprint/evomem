# Context propagation

A request's deadline and cancellation travel only as far as its
`context.Context` is passed. One `context.Background()` in the middle of a call
chain is where they stop.

| #   | Check                                                                                                   | How                                                                     | Source                                                       |
| --- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------ |
| CP1 | Every function that performs I/O takes `ctx context.Context` as its first parameter                     | read repository, client and service signatures                          | Go 1.27 — package context                                    |
| CP2 | `context.Background()` and `context.TODO()` appear only in `main`, tests and top-level workers          | grep both outside those files                                           | Go 1.27 — package context; golangci-lint 2.14 — contextcheck |
| CP3 | Outbound HTTP requests are built with `http.NewRequestWithContext`                                      | golangci-lint `noctx`                                                   | golangci-lint 2.14 — noctx                                   |
| CP4 | Database calls use the `Context` variants (`QueryContext`, `ExecContext`, `BeginTx`) or pgx equivalents | grep `.Query(`, `.Exec(`, `.Begin(` on `*sql.DB` and `*sql.Tx`          | Go 1.27 — database/sql                                       |
| CP5 | Every `WithTimeout`, `WithDeadline` and `WithCancel` is followed by `defer cancel()`                    | `go vet` (`lostcancel`)                                                 | Go 1.27 — cmd/vet, lostcancel                                |
| CP6 | No struct stores a `context.Context`                                                                    | golangci-lint `containedctx`                                            | golangci-lint 2.14 — containedctx; Go 1.27 — package context |
| CP7 | No context is re-wrapped inside a loop                                                                  | golangci-lint `fatcontext`                                              | golangci-lint 2.14 — fatcontext                              |
| CP8 | Gin handlers pass `c.Request.Context()` downstream, and goroutines receive `c.Copy()`                   | grep handlers for `(c,` passed as a context and for `go func` using `c` | Gin 1.12 — goroutines inside a middleware                    |
| CP9 | Log calls inside a request use the `...Context` slog methods                                            | golangci-lint `sloglint` with `context: scope`                          | Go 1.27 — log/slog; golangci-lint 2.14 — sloglint            |

## Why each one

**CP2** is where most propagation breaks. A helper that calls
`context.Background()` because it had no context to hand keeps running its
query after the client has gone and the handler has returned, holding a pooled
connection the whole time.

**CP5** is a leak that `go vet` catches for free. A context from `WithTimeout`
that is never cancelled keeps its timer and its goroutine until the deadline,
once per request.

**CP8** is specific to Gin. The `*gin.Context` is recycled after the handler
returns; a goroutine still holding it reads another request's values. The Gin
documentation says the original context must not be used from a new goroutine,
and gives `c.Copy()` for that case.
