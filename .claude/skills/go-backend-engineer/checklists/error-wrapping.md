# Error wrapping

Go errors are values, and the chain is the diagnosis. Wrap with context at each
layer, match by identity at the edge, and map to HTTP in exactly one place.

| #    | Check                                                                                         | How                                                                                    | Source                                                   |
| ---- | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| EW1  | Errors are wrapped with `%w` and a description of the operation                               | golangci-lint `errorlint` with `errorf` enabled; grep `Errorf(` with `%v` and an `err` | Go 1.27 — package errors; golangci-lint 2.14 — errorlint |
| EW2  | Errors are matched with `errors.Is` / `errors.As`, never `==` or a type assertion             | golangci-lint `errorlint` (`comparison`, `asserts`)                                    | Go 1.27 — package errors                                 |
| EW3  | Every returned error is checked                                                               | golangci-lint `errcheck`                                                               | golangci-lint 2.14 — errcheck                            |
| EW4  | Errors from other packages are wrapped at the boundary where they enter this one              | golangci-lint `wrapcheck`                                                              | golangci-lint 2.14 — wrapcheck                           |
| EW5  | Domain failures are sentinel or typed errors declared in the domain package                   | read the package's `var Err...` and error types                                        | Go 1.27 — package errors                                 |
| EW6  | One function maps domain errors to `application/problem+json` with a stable `type`            | find the mapper; every handler returns its error to it                                 | RFC 9457 §3                                              |
| EW7  | An unmapped error is a 500 with a generic `detail`; the wrapped chain is logged, not returned | return an unexpected error in a test; assert on body and log                           | OWASP ASVS 5.0.0 V16; RFC 9457 §5                        |
| EW8  | A recovery middleware turns a panic into a 500 and logs the stack                             | panic in a test handler; assert the response and the log                               | Go 1.27 — net/http Handler                               |
| EW9  | Logs are `slog` JSON; secrets implement `slog.LogValuer` or are removed in `ReplaceAttr`      | read the logger construction; grep `log.Printf` and `fmt.Println` in non-test code     | Go 1.27 — log/slog; OWASP ASVS 5.0.0 V16                 |
| EW10 | The request id and trace id from `traceparent` are on every log line of a request             | send a request with `traceparent`; find the id in each line                            | W3C Trace Context Level 1; Go 1.27 — log/slog            |

## Why each one

**EW2** fails the moment EW1 is followed. `err == sql.ErrNoRows` works until a
repository wraps the error with context, and then the not-found path becomes a 500. `errors.Is` walks the chain; `==` does not.

**EW1** is what makes a log line readable. `load order 42: query orders:
context deadline exceeded` tells the on-call engineer which call, which
record and why. A bare `context deadline exceeded` tells them nothing.

**EW7** keeps that chain out of the response. The same text that helps the
engineer tells an attacker the table name.
