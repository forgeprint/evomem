# Go Senior Software Architect

## What it changes

An agent writing Go without this expert tends to write Go-shaped Java: an
interface beside every struct, a `models` package, a service that stores its
context, errors compared by message, goroutines started and forgotten. It
compiles, and each of those is a design that gets harder to change with every
file added to it.

This expert holds a Go module to the rules its own toolchain and standard
library already follow:

- **Four decisions first** — one module or several, the package map, the Go
  version floor, and who owns long-lived goroutines.
- **Packages as the architecture** — `internal/` for everything not
  deliberately public, a `main` that only wires, no grab-bag packages, cycles
  broken by moving types rather than merging packages.
- **Gates pinned in `go.mod`** with the `tool` directive: `go vet` in full,
  staticcheck, govulncheck, `go mod tidy -diff`, the race detector, and
  depguard for import direction.
- **Interfaces owned by their consumers**, constructors that return concrete
  types.
- **Context passed, never stored**, with the Gin-specific rules for
  `c.Request.Context()` and `c.Copy()`.
- **Errors wrapped deliberately** and matched with `errors.Is` / `As` /
  `AsType`, never by text.
- **Every goroutine owned and ended**, with graceful shutdown and the Go 1.27
  goroutine leak profile.

## What it fits

- Starting a Go service, CLI or library, before the package map is public.
- Reviewing a Go codebase's structure, or a change that adds a package, a
  goroutine or an exported API.
- Go 1.26 and 1.27. Most rules hold on older versions; `errors.AsType`
  needs 1.26, `sync.WaitGroup.Go` 1.25, the `tool` directive 1.24.

## What it does not fit

- **Performance work** — escape analysis, allocation profiles, GC tuning:
  `performance-engineer`. This expert asks whether goroutines end, not how
  fast they run.
- **Security** — `govulncheck` is run here; what to do about its findings is
  `security-reviewer`'s question.
- **cgo, embedded targets and WebAssembly** — the layout and error rules
  transfer; the concurrency and shutdown rules assume a server process.
- **A single-file tool.** `internal/` and depguard rules for a `main.go` of
  two hundred lines are ceremony, and this expert will say so.

## Pros and cons

**In its favour:** almost every rule is one the Go project itself states —
in the code review guidance, the `context` documentation, the modules
reference — so the expert is closer to the language's own conventions than to
anybody's taste, and the linters named here already implement most of them.

**Against it:** it is firm about consumer-owned interfaces, which reads as
unfamiliar to a team arriving from Java or C# and costs some explaining. And
the `%w` rule (EW5) asks for judgement on every wrap, which is slower than
wrapping everything by habit — deliberately.
