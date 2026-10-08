# Context flow

`context.Context` carries cancellation, deadlines and request-scoped values
down a call tree. It works only if it is passed, never stored, and always
cancelled by whoever created it.

| #   | Check                                                                          | How                                                              | Source                               |
| --- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------------ |
| CF1 | Every function that does I/O or may block takes `ctx context.Context` first    | read exported signatures; golangci-lint `noctx` for HTTP         | `context` package documentation      |
| CF2 | No struct stores a `context.Context`                                           | golangci-lint `containedctx`                                     | `context` package documentation      |
| CF3 | `context.Background()` and `context.TODO()` appear only in `main` and in tests | `grep -rn "context.Background()\|context.TODO()" --include=*.go` | `context` package documentation      |
| CF4 | Every derived context's `cancel` is called                                     | `go vet` `lostcancel`                                            | cmd/vet — lostcancel                 |
| CF5 | Outbound HTTP requests are built with `http.NewRequestWithContext`             | golangci-lint `noctx`                                            | `net/http` documentation             |
| CF6 | Database calls use the `...Context` variants                                   | `grep -rnE "\.(Query\|Exec\|QueryRow)\(" --include=*.go`         | `database/sql` documentation         |
| CF7 | Context values carry request-scoped data only, under unexported key types      | read each `context.WithValue`                                    | `context` package documentation      |
| CF8 | Gin: downstream calls get `c.Request.Context()`, not the `*gin.Context`        | read handlers and the services they call                         | Gin — Context and cancellation       |
| CF9 | Gin: a goroutine started in a handler or middleware uses `c.Copy()`            | `grep -rn "go func" ` in handler packages; check for `Copy()`    | Gin — Goroutines inside a middleware |

## Why each one

**CF2** is stated in the `context` documentation itself: a stored context
outlives the request it belonged to, so a later call uses a cancelled or
wrong deadline and fails in a way that looks random.

**CF6** because `db.Query` without a context cannot be cancelled. A client that
gave up still holds a connection for as long as the query runs, and under load
the pool empties.

**CF9** is Gin-specific: Gin pools `gin.Context` objects and hands the same
one to another request after the handler returns. A goroutine still holding
it reads and writes another user's request.
