# References

Every rule in [`SKILL.md`](SKILL.md) and every checklist row rests on one of
these. An item that cites nothing is somebody's opinion and does not belong in
a review ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

The Go releases come from `go.dev/dl`. Module versions come from the Go module
proxy (`proxy.golang.org/<module>/@latest`). Linter and rule IDs come from each
project's source or documentation at the version shown. Everything was read on
the date shown. **Re-check every 90 days**, and when Go 1.28 ships (expected in
February 2027, on the six-month release cadence).

> **Next re-check due: 2026-12-23.**

## Go toolchain and standard library

| Reference                                                                                                                   | Version                                                                                                                 | Checked    | Used for                                                          |
| --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ---------- | ----------------------------------------------------------------- |
| [Go release history](https://go.dev/doc/devel/release)                                                                      | 1.27.1 and 1.26.8 are the supported releases                                                                            | 2026-09-24 | The toolchain in SKILL.md                                         |
| [Go 1.27 release notes](https://go.dev/doc/go1.27)                                                                          | 1.27; `goroutineleak` profile generally available; `encoding/json` backed by the v2 implementation with the v1 API kept | 2026-09-24 | `goroutine-lifecycle.md` GL10; ST7                                |
| [Go 1.25 release notes](https://go.dev/doc/go1.25)                                                                          | 1.25; `testing/synctest` generally available, `sync.WaitGroup.Go`                                                       | 2026-09-24 | GL2, `table-tests.md` TT8                                         |
| [package context](https://pkg.go.dev/context)                                                                               | go1.27.1                                                                                                                | 2026-09-24 | `context-propagation.md` CP1, CP2, CP6; GL3                       |
| [package net/http](https://pkg.go.dev/net/http)                                                                             | go1.27.1; zero `Server` timeouts and zero `Client.Timeout` mean none                                                    | 2026-09-24 | `server-and-client-timeouts.md` ST1, ST2, ST4, ST6; GL5, GL6; EW8 |
| [package net/http/httptest](https://pkg.go.dev/net/http/httptest)                                                           | go1.27.1                                                                                                                | 2026-09-24 | TT4                                                               |
| [package database/sql](https://pkg.go.dev/database/sql)                                                                     | go1.27.1; max open 0 (unlimited), max idle 2, no lifetime by default                                                    | 2026-09-24 | `sql-pool.md` SP1, SP3, SP5, SP6, SP7, SP9; CP4; TT7              |
| [package errors](https://pkg.go.dev/errors)                                                                                 | go1.27.1                                                                                                                | 2026-09-24 | `error-wrapping.md` EW1, EW2, EW5; SP9                            |
| [package log/slog](https://pkg.go.dev/log/slog)                                                                             | go1.27.1; `LogValuer`, `HandlerOptions.ReplaceAttr`, `...Context` methods                                               | 2026-09-24 | EW9, EW10, CP9                                                    |
| [os/signal.NotifyContext](https://pkg.go.dev/os/signal#NotifyContext)                                                       | go1.27.1                                                                                                                | 2026-09-24 | GL4                                                               |
| [encoding/json — Decoder.DisallowUnknownFields](https://pkg.go.dev/encoding/json#Decoder.DisallowUnknownFields)             | go1.27.1                                                                                                                | 2026-09-24 | ST7                                                               |
| [package testing/synctest](https://pkg.go.dev/testing/synctest)                                                             | go1.27.1                                                                                                                | 2026-09-24 | TT8                                                               |
| [package math/rand/v2](https://pkg.go.dev/math/rand/v2)                                                                     | go1.27.1                                                                                                                | 2026-09-24 | GL8 — jitter                                                      |
| [cmd/vet](https://pkg.go.dev/cmd/vet) and [lostcancel](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel) | go1.27.1                                                                                                                | 2026-09-24 | CP5                                                               |
| [Data race detector](https://go.dev/doc/articles/race_detector)                                                             | current at check                                                                                                        | 2026-09-24 | TT5                                                               |
| [Effective Go — goroutines](https://go.dev/doc/effective_go#goroutines)                                                     | current at check                                                                                                        | 2026-09-24 | GL1                                                               |
| [Go wiki — TableDrivenTests](https://go.dev/wiki/TableDrivenTests)                                                          | current at check                                                                                                        | 2026-09-24 | TT1                                                               |

## Modules

| Reference                                                                             | Version                                                                            | Checked    | Used for      |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- | ---------- | ------------- |
| [Gin — documentation](https://github.com/gin-gonic/gin/blob/v1.12.0/docs/doc.md)      | v1.12.0; `BindJSON` aborts with a `text/plain` 400; goroutines must use `c.Copy()` | 2026-09-24 | CP8, ST9, GL1 |
| [Gin — binding](https://pkg.go.dev/github.com/gin-gonic/gin/binding)                  | v1.12.0; `EnableDecoderDisallowUnknownFields` defaults to false                    | 2026-09-24 | ST7           |
| [validator v10](https://pkg.go.dev/github.com/go-playground/validator/v10)            | v10.30.5                                                                           | 2026-09-24 | ST8           |
| [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)                         | pgx v5.11.0; `pool_max_conns` defaults to max(4, NumCPU)                           | 2026-09-24 | SP2           |
| [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup)                             | golang.org/x/sync v0.23.0                                                          | 2026-09-24 | GL2           |
| [goleak](https://pkg.go.dev/go.uber.org/goleak)                                       | v1.3.0; `VerifyNone` is incompatible with `t.Parallel`, `VerifyTestMain` is not    | 2026-09-24 | GL9           |
| [testcontainers-go — PostgreSQL](https://golang.testcontainers.org/modules/postgres/) | v0.44.0; `postgres.Run(ctx, image, ...)`                                           | 2026-09-24 | TT6           |

## Static analysis

| Reference                                                                             | Version                                                                        | Checked    | Used for                                                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [golangci-lint — linters](https://golangci-lint.run/docs/linters/)                    | v2.14.0, released 2026-09-24                                                   | 2026-09-24 | `noctx`, `contextcheck`, `containedctx`, `fatcontext`, `sloglint`, `bodyclose`, `musttag`, `errorlint`, `errcheck`, `wrapcheck`, `sqlclosecheck`, `rowserrcheck` — CP2, CP3, CP6, CP7, CP9, ST5, ST10, EW1–EW4, SP4, SP5, TT9 |
| [gosec — rule list](https://github.com/securego/gosec/blob/v2.29.0/rules/rulelist.go) | v2.29.0 (rule IDs read at this tag; golangci-lint bundles its own gosec build) | 2026-09-24 | G112 (ST1), G114 (ST3), G201 and G202 (SP8)                                                                                                                                                                                   |
| [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)                   | golang.org/x/vuln v1.8.0                                                       | 2026-09-24 | TT9                                                                                                                                                                                                                           |

## HTTP and operations

| Reference                                                                                                               | Version                                       | Checked    | Used for                                                                                           |
| ----------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------- |
| [RFC 9110 — HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110)                                                     | June 2022, Internet Standard                  | 2026-09-24 | §9.2.2 idempotent methods — SP10, GL8                                                              |
| [RFC 9457 — Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457)                                      | July 2023, obsoletes RFC 7807                 | 2026-09-24 | §3 — ST9, EW6, TT3; §5 — EW7                                                                       |
| [The Idempotency-Key HTTP Header Field](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/07/) | draft-07, 2025-10-15, **expired**             | 2026-09-24 | SP10. The clearest description of the convention; it never became a standard, and is cited as such |
| [W3C Trace Context](https://www.w3.org/TR/trace-context/)                                                               | Level 1, W3C Recommendation                   | 2026-09-24 | EW10                                                                                               |
| [Kubernetes — Pod lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)                         | Kubernetes 1.37; grace period 30 s by default | 2026-09-24 | GL5, GL7                                                                                           |

## Tracked in `docs/review-standards.md`

Cited by chapter or category in the checklists and not restated here.

| Standard                  | Version in `docs/review-standards.md` | Rows that cite it       |
| ------------------------- | ------------------------------------- | ----------------------- |
| OWASP ASVS                | 5.0.0 — V1, V2, V16                   | ST8, EW7, EW9, SP8, TT2 |
| OWASP API Security Top 10 | 2023 — API3, API4                     | ST6, ST7, TT2           |

The security review itself belongs to [`security-reviewer`](../security-reviewer/SKILL.md).
