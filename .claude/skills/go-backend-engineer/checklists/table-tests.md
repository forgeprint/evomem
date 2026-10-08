# Table tests

Go's testing package has no framework to hide behind: a table, `t.Run`, and
`httptest`. The rows below are about making those tests find the bugs this
expert is written against — races, leaks, and SQL that only fails on a real
database.

| #   | Check                                                                                                 | How                                                          | Source                                                               |
| --- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------------------- |
| TT1 | Handler and validation tests are table-driven, one `t.Run` per case, with the case name in the output | read the test files                                          | Go wiki — TableDrivenTests                                           |
| TT2 | Each table for a request type includes an unknown field, an oversized body and each bound exceeded    | read the table rows                                          | OWASP ASVS 5.0.0 V2; OWASP API4:2023                                 |
| TT3 | Each domain error has a case asserting status, `type` and `application/problem+json`                  | count the cases against the error declarations               | RFC 9457 §3                                                          |
| TT4 | Handlers are exercised through `httptest.NewRecorder` or `httptest.NewServer`                         | read the handler tests                                       | Go 1.27 — net/http/httptest                                          |
| TT5 | CI runs `go test -race ./...`                                                                         | read the CI job                                              | Go 1.27 — data race detector                                         |
| TT6 | Repository tests use PostgreSQL through testcontainers-go, not a mock of `database/sql`               | grep `postgres.Run(` from the testcontainers postgres module | testcontainers-go 0.44                                               |
| TT7 | A test forces a failure after the first statement of a transaction and asserts nothing committed      | find the rollback test                                       | Go 1.27 — database/sql Tx                                            |
| TT8 | Timeout, retry and backoff tests run inside `synctest.Test`, not with real sleeps                     | grep tests for `time.Sleep`; look for `testing/synctest`     | Go 1.25 release notes — testing/synctest; Go 1.27 — testing/synctest |
| TT9 | `go vet`, `golangci-lint run` and `govulncheck ./...` run in CI and fail it                           | read the CI job                                              | golangci-lint 2.14; govulncheck 1.8                                  |

## Why each one

**TT5** is the only reliable way to find the races GL1 describes. The race
detector reports a race only when it happens during the run, so it has to be on
for every run, not for the one somebody remembers.

**TT6** is what makes SP4 to SP10 testable. A mocked `database/sql` has no pool,
no transactions and no unique constraints, so a leaked connection, a partial
commit or a replayed write all pass.

**TT8** removes the slowest and flakiest tests. `synctest` runs the code under
a fake clock that advances only when every goroutine in the bubble is blocked,
so a thirty-second backoff test takes microseconds and gives the same result
every time.
