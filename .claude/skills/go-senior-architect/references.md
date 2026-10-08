# References

Every rule in [`SKILL.md`](SKILL.md) and every checklist row rests on one of
these. An item that cites nothing is somebody's opinion and does not belong in
a review ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

Each row carries the version current when it was read and the date of that
reading. Go versions come from `go.dev/dl`, module versions from the Go module
proxy, both on the date shown. **Re-check every 90 days**, and when Go 1.28
ships (expected February 2027): Go supports the two newest releases, so that
release ends support for 1.26 and moves the floor in PL7.

> **Next re-check due: 2026-12-23.**

## Language and toolchain

| Reference                                                            | Version                                       | Checked    | Used for                                                                          |
| -------------------------------------------------------------------- | --------------------------------------------- | ---------- | --------------------------------------------------------------------------------- |
| [Go release history and policy](https://go.dev/doc/devel/release)    | go1.27.1 (2026-09-01); 1.26.8 still supported | 2026-09-24 | The target in SKILL.md; PL7                                                       |
| [Go 1.27 release notes](https://go.dev/doc/go1.27)                   | 1.27, released 2026-08-19                     | 2026-09-24 | TG3 (`stdversion` under `go test`), GL7 (`goroutineleak` profile GA)              |
| [Go 1.26 release notes](https://go.dev/doc/go1.26)                   | 1.26                                          | 2026-09-24 | EW2 (`errors.AsType`), TG8 (`go fix` modernizers)                                 |
| [Go 1.25 release notes](https://go.dev/doc/go1.25)                   | 1.25                                          | 2026-09-24 | GL2 — `sync.WaitGroup.Go`                                                         |
| [The Go programming language specification](https://go.dev/ref/spec) | Go 1.27                                       | 2026-09-24 | IP6 (type parameters), GL4 (`close`), the import-cycle rule in SKILL.md §2        |
| [Go modules reference](https://go.dev/ref/mod)                       | Go 1.27                                       | 2026-09-24 | PL6–PL8, TG1 — `go`, `toolchain`, `replace`, and the `tool` directive (Go 1.24)   |
| [cmd/go](https://pkg.go.dev/cmd/go)                                  | Go 1.27                                       | 2026-09-24 | PL1 (internal directories), TG2 (`go mod tidy -diff`), TG3, TG9 (`go mod verify`) |
| [Organizing a Go module](https://go.dev/doc/modules/layout)          | current at check                              | 2026-09-24 | PL1, PL2 — `cmd/`, `internal/`, server and library layouts                        |
| [cmd/vet](https://pkg.go.dev/cmd/vet)                                | Go 1.27                                       | 2026-09-24 | TG3, CF4 — the analyzers, including `lostcancel` and `copylocks`                  |
| [Data race detector](https://go.dev/doc/articles/race_detector)      | current at check                              | 2026-09-24 | TG6, GL6                                                                          |

## Standard library

| Reference                                                 | Version | Checked    | Used for                                                               |
| --------------------------------------------------------- | ------- | ---------- | ---------------------------------------------------------------------- |
| [`context`](https://pkg.go.dev/context)                   | Go 1.27 | 2026-09-24 | SKILL.md §5; CF1–CF3, CF7, GL3 — first parameter, never stored, values |
| [`errors`](https://pkg.go.dev/errors)                     | Go 1.27 | 2026-09-24 | EW2, EW3 — `Is`, `As`, `AsType`                                        |
| [`net/http`](https://pkg.go.dev/net/http)                 | Go 1.27 | 2026-09-24 | CF5, GL5 — `NewRequestWithContext`, `Server.Shutdown`                  |
| [`database/sql`](https://pkg.go.dev/database/sql)         | Go 1.27 | 2026-09-24 | CF6 — the `Context` method variants                                    |
| [`os/signal`](https://pkg.go.dev/os/signal)               | Go 1.27 | 2026-09-24 | GL5 — `NotifyContext`                                                  |
| [`testing/synctest`](https://pkg.go.dev/testing/synctest) | Go 1.27 | 2026-09-24 | GL9                                                                    |

## Guidance

| Reference                                                                                 | Version                | Checked    | Used for                                                                |
| ----------------------------------------------------------------------------------------- | ---------------------- | ---------- | ----------------------------------------------------------------------- |
| [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)                         | wiki, current at check | 2026-09-24 | SKILL.md §4; IP1–IP3, IP7 (interfaces), PL5, GL1 (goroutine lifetimes)  |
| [Effective Go](https://go.dev/doc/effective_go)                                           | current at check       | 2026-09-24 | PL4, PL9 (`init`), IP4, IP5 (interface checks), GL4, EW8                |
| [Go blog — Package names](https://go.dev/blog/package-names)                              | published 2015-02-04   | 2026-09-24 | PL3, PL4                                                                |
| [Go blog — Working with errors in Go 1.13](https://go.dev/blog/go1.13-errors)             | published 2019-10-17   | 2026-09-24 | SKILL.md §6; EW1, EW4, EW5 — `%w`, and wrapping as an API decision      |
| [Google Go style — best practices](https://google.github.io/styleguide/go/best-practices) | current at check       | 2026-09-24 | PL3 (util packages), PL9, GL8 (global state), EW6 (logging errors), IP6 |
| [Go proverbs](https://go-proverbs.github.io/)                                             | 2015 talk              | 2026-09-24 | IP4 — "the bigger the interface, the weaker the abstraction"            |

## Tools that enforce the rules

| Reference                                                                                | Version                              | Checked    | Used for                                                                                                                                              |
| ---------------------------------------------------------------------------------------- | ------------------------------------ | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| [staticcheck](https://staticcheck.dev/docs/)                                             | 2026.2.1 (module v0.8.1), 2026-08-21 | 2026-09-24 | SKILL.md §3; TG4                                                                                                                                      |
| [govulncheck](https://go.dev/doc/security/vuln/)                                         | golang.org/x/vuln v1.8.0, 2026-09-08 | 2026-09-24 | TG5                                                                                                                                                   |
| [golangci-lint — linters](https://golangci-lint.run/docs/linters/)                       | v2.14.0, 2026-09-24                  | 2026-09-24 | TG7 (`depguard`), CF1/CF5 (`noctx`), CF2 (`containedctx`), EW1/EW2 (`errorlint`), EW7 (`wrapcheck`), PL9 (`gochecknoinits`), GL8 (`gochecknoglobals`) |
| [golangci-lint — linter settings](https://golangci-lint.run/docs/linters/configuration/) | v2.14.0                              | 2026-09-24 | TG7 — `version: "2"`, `depguard.rules`, `deny` with `desc`                                                                                            |
| [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup)                                | golang.org/x/sync v0.23.0            | 2026-09-24 | SKILL.md §7; GL2                                                                                                                                      |

## Frameworks

| Reference                                                                                                        | Version                  | Checked    | Used for                                 |
| ---------------------------------------------------------------------------------------------------------------- | ------------------------ | ---------- | ---------------------------------------- |
| [Gin — Context and cancellation](https://gin-gonic.com/en/docs/server-config/context/)                           | gin v1.12.0 (2026-02-28) | 2026-09-24 | SKILL.md §5; CF8 — `c.Request.Context()` |
| [Gin — Goroutines inside a middleware](https://gin-gonic.com/en/docs/middleware/goroutines-inside-a-middleware/) | gin v1.12.0              | 2026-09-24 | CF9 — `c.Copy()`, and why                |

## Architecture

| Reference                                                                                                                  | Version          | Checked    | Used for                                                      |
| -------------------------------------------------------------------------------------------------------------------------- | ---------------- | ---------- | ------------------------------------------------------------- |
| [Architecture decision records — Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) | original, 2011   | 2026-09-24 | SKILL.md §1; Alternatives is this catalog's addition          |
| [C4 model](https://c4model.com/)                                                                                           | current at check | 2026-09-24 | SKILL.md §9 — Context and Container, one container per binary |

## Deferred to elsewhere

- Authentication, input handling, secrets, and the triage of what `govulncheck`
  reports: [`docs/review-standards.md`](../../docs/review-standards.md) and
  [`security-reviewer`](../security-reviewer/SKILL.md).
- Build pipelines, images, supply chain: the NIST SSDF and SLSA rows in
  `docs/review-standards.md`, and [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md).
