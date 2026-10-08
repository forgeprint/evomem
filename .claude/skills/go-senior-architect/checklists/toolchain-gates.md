# Toolchain gates

Go ships most of its architecture checks with the toolchain. The work is
pinning them, running all of them, and making any output a failure.

| #   | Check                                                                        | How                                                                      | Source                                   |
| --- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ---------------------------------------- |
| TG1 | Linters are pinned with `tool` directives in `go.mod`, not installed ad hoc  | `grep -n "^tool" go.mod`; run as `go tool <name>`                        | Go modules reference — `tool` (Go 1.24)  |
| TG2 | `go mod tidy -diff` is empty in CI                                           | the CI script; it exits non-zero on a diff                               | cmd/go — `go mod tidy`                   |
| TG3 | `go vet ./...` runs in full, not only the subset `go test` runs              | the CI script                                                            | cmd/go — `go vet`; Go 1.27 release notes |
| TG4 | staticcheck runs with its default checks and fails the build                 | `go tool staticcheck ./...`                                              | staticcheck 2026.2                       |
| TG5 | govulncheck runs on every change, and on a schedule                          | `go tool govulncheck ./...`                                              | govulncheck 1.8                          |
| TG6 | Tests run with the race detector                                             | `go test -race ./...`                                                    | Go — Data race detector                  |
| TG7 | Import direction rules are written as depguard rules with a `desc`           | `.golangci.yml` has `version: "2"` and `linters.settings.depguard.rules` | golangci-lint 2.14 — depguard            |
| TG8 | `go fix` modernizers are run before a Go version bump, and the diff reviewed | `go fix ./...` on the upgrade branch                                     | Go 1.26 release notes — `go fix`         |
| TG9 | `go mod verify` passes in CI                                                 | the CI script                                                            | cmd/go — `go mod verify`                 |

## Why each one

**TG1** makes the linter version part of the module. Without it, two
developers and CI run three versions of staticcheck and argue about findings
that only one of them sees.

**TG3** is easy to miss: `go test` runs a high-confidence subset of vet
(`atomic`, `bools`, `errorsas`, `printf`, `stdversion` and a few more). The
analyzers that catch architectural mistakes — `lostcancel`, `copylocks` — only
run under `go vet` itself.

**TG7** is the only row that encodes direction rather than visibility.
`internal/` says who may import a package; depguard says what a package may
import, which is the question a layering rule asks.
