# Package layout

In Go the package is the only unit of encapsulation, and `internal/` is the
only way to say "not for you" that the toolchain enforces. The layout is the
architecture.

| #   | Check                                                                                                                             | How                                                                         | Source                                                |
| --- | --------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ----------------------------------------------------- |
| PL1 | Every package that is not a deliberate public API is under `internal/`                                                            | `go list ./...`; each path outside `internal/` and `cmd/` is justified      | cmd/go — Internal Directories; Organizing a Go module |
| PL2 | `main` only wires and calls `run`; with more than one binary, each is `cmd/<name>/main.go`                                        | read each `main.go`; no business branches                                   | Organizing a Go module                                |
| PL3 | No package named `util`, `common`, `helpers`, `misc`, `models` or `types`                                                         | `go list ./... \| grep -E "/(util\|common\|helpers\|misc\|models\|types)$"` | Go blog — Package names; Google Go style — naming     |
| PL4 | Package names are short, lower-case, and not repeated in their identifiers (`order.Order` is fine, `order.OrderService` stutters) | read exported identifiers                                                   | Go blog — Package names; Effective Go — package names |
| PL5 | No cycle was "fixed" by merging two packages or by an `interface{}` / `any` parameter                                             | read the history of the last cycle-breaking change                          | Go Code Review Comments — interfaces                  |
| PL6 | One `go.mod` unless a separately versioned module is recorded in an ADR                                                           | `find . -name go.mod`                                                       | Go modules reference                                  |
| PL7 | The `go` directive is the real floor, and CI tests that version                                                                   | read `go.mod`; compare with the CI matrix                                   | Go modules reference — `go` and `toolchain`           |
| PL8 | No `replace` directive in a module other modules import                                                                           | `grep -n "^replace" go.mod`                                                 | Go modules reference — `replace`                      |
| PL9 | No `init()` that does I/O, reads configuration or registers global state                                                          | `grep -rn "func init()" --include=*.go`; golangci-lint `gochecknoinits`     | Effective Go — init; Google Go style — global state   |

## Why each one

**PL1** is the cheapest decision in this list and the most expensive to get
wrong. A package outside `internal/` can be imported by any module, and once
one does, moving or changing it is a breaking change to somebody else.

**PL3** because a grab-bag package is imported by everything and grows to
import everything, and that is where the first import cycle appears — which
Go does not allow to compile.

**PL8** because `replace` directives apply only in the main module. A library
that relies on one builds against something its consumers never see.
