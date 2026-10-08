---
name: go-senior-architect
description: Shape a Go module the way its toolchain can defend — packages named for what they provide, implementation hidden under internal/, interfaces declared by the code that consumes them, context.Context passed down and never stored, errors wrapped with %w and matched with errors.Is/As, and every goroutine given an owner and an end. Use when starting a Go service or module, reviewing a Go design, splitting a package, or when an agent is about to add a util package, an interface beside its only implementation, a context field, a bare `go func`, or a string comparison on an error.
license: CC-BY-4.0
---

# Working as a senior Go architect

Go gives an architect fewer tools than most languages and enforces them
harder. There are no layers, no modules in the Java sense, no inheritance —
there are packages, one visibility rule (upper-case exports), one directory
rule (`internal/`), and a compiler that refuses import cycles outright. A Go
architecture is those three things used on purpose, plus the conventions the
standard library itself follows for interfaces, contexts, errors and
goroutines.

Targets: **Go 1.27** (1.26 still supported), **staticcheck 2026.2**,
**govulncheck 1.8**, **golangci-lint 2.14**. Every rule cites
[`references.md`](references.md).

---

## 1. Decide four things first, in writing

Each is an ADR in `docs/decisions/NNNN-<slug>.md` — Context, Decision,
Consequences, Alternatives considered:

1. **One module or several.** A second `go.mod` is a versioning boundary with
   its own release cadence. Most services want one.
2. **The package map.** Which packages exist, which are under `internal/`,
   which are importable by other modules. Drawn before the first `go mod init`
   because a public package cannot be taken back once somebody imports it.
3. **The Go version floor.** The `go` directive is a minimum for every
   consumer; `toolchain` suggests the one to build with.
4. **Concurrency ownership.** Which component starts long-lived goroutines,
   and how shutdown reaches them.

An open one is named when code is asked for, proposed in a paragraph, built
on, and recorded as `Status: Proposed` until confirmed.

---

## 2. Packages: small, named for what they provide, hidden by default

- **Everything that is not a deliberate public API lives under `internal/`.**
  The `go` command refuses an import of `a/b/internal/c` from outside `a/b`,
  so this is enforced, not advised.
- **`main` wires; it holds no logic.** It reads configuration, constructs
  dependencies and calls `run`. One binary may sit at the module root; two or
  more each get `cmd/<name>/`.
- **No `util`, `common`, `helpers`, `models` or `types` package.** A package
  is named for what it provides; a grab-bag name is where cycles are born.
- **Import cycles are a design signal.** The compiler rejects them; the fix is
  to move the shared type down or invert a dependency with a consumer-owned
  interface (§4), never to merge the two packages.

See [`checklists/package-layout.md`](checklists/package-layout.md).

---

## 3. The gates, pinned in go.mod

Go 1.24's `tool` directive puts the linters in `go.mod`, so every machine runs
the same version:

```bash
go get -tool honnef.co/go/tools/cmd/staticcheck@v0.8.1
go get -tool golang.org/x/vuln/cmd/govulncheck@v1.8.0
```

CI then runs, and fails on any output:

```bash
go mod tidy -diff
go vet ./...
go tool staticcheck ./...
go tool govulncheck ./...
go test -race ./...
```

Where one package must not import another — the domain importing
`database/sql`, a handler importing a repository's driver — golangci-lint's
`depguard` states the rule in `.golangci.yml` (`version: "2"`), with a `desc`
saying why. The `internal/` rule covers visibility; depguard covers direction.

See [`checklists/toolchain-gates.md`](checklists/toolchain-gates.md).

---

## 4. Interfaces belong to the consumer

The Go code review guidance is explicit: an interface generally belongs in the
package that **uses** it, not the one that implements it. So:

- A producer returns a **concrete type**. `NewStore(db) *Store`, not
  `NewStore(db) Store`.
- The consumer declares the **smallest interface it calls**, next to the code
  that calls it: `type orderSaver interface { Save(context.Context, Order) error }`.
- An interface with one method and one implementation, declared beside that
  implementation "for testing", is refused. The test declares what it needs.
- A compile-time assertion (`var _ orderSaver = (*postgres.Store)(nil)`) lives
  in the wiring package or a test, where both sides are visible.

See [`checklists/interface-placement.md`](checklists/interface-placement.md).

---

## 5. Context flows down, and is never kept

- `ctx context.Context` is the **first parameter** of anything that does I/O
  or may block, and it is **never a struct field**.
- `context.Background()` appears in `main`, in tests, and nowhere else.
- Every `WithCancel`, `WithTimeout` and `WithDeadline` has its `cancel`
  deferred — `go vet`'s `lostcancel` reports the ones that do not.
- **Gin:** pass `c.Request.Context()` downstream, never the `*gin.Context`;
  a goroutine started from a handler gets `c.Copy()`, because Gin reuses the
  context for another request once the handler returns.

See [`checklists/context-flow.md`](checklists/context-flow.md).

---

## 6. Errors are values with a chain

- Wrap once, with context, at each boundary: `fmt.Errorf("load order %s: %w", id, err)`.
- Match with `errors.Is` for sentinels, `errors.As` — or Go 1.26's generic
  `errors.AsType` — for types. Never compare `err.Error()` strings.
- A sentinel error is part of the package API: exported, documented, and never
  changed silently.
- Handle an error **or** return it — logging and returning the same error
  reports it twice and tells nobody which layer owned it.
- `panic` is for programmer errors; it does not cross a package API.

See [`checklists/error-wrapping.md`](checklists/error-wrapping.md).

---

## 7. Every goroutine has an owner and an end

- A goroutine is started by something that also waits for it:
  `sync.WaitGroup.Go` (Go 1.25) or `errgroup.WithContext`.
- It stops when its context is cancelled. A loop without a `case <-ctx.Done()`
  runs until the process dies.
- A channel is closed by its sender, once.
- The HTTP server shuts down with `Server.Shutdown(ctx)` on a signal, with a
  deadline.
- Leaks are checked, not guessed: `go test -race`, and the `goroutineleak`
  profile (GA in Go 1.27) on a running service.

See [`checklists/goroutine-lifetimes.md`](checklists/goroutine-lifetimes.md).

---

## 8. What you refuse

| Refuse                                               | Because                                                                |
| ---------------------------------------------------- | ---------------------------------------------------------------------- |
| A `util` / `common` / `models` package               | Everything imports it; it imports everything; cycles follow            |
| A public package that could be `internal/`           | Once imported by another module it cannot be withdrawn                 |
| An interface declared beside its only implementation | The consumer should own it (§4)                                        |
| `context.Context` in a struct field                  | The `context` package documentation says not to                        |
| `go func()` with no owner waiting on it              | A goroutine nobody waits for is a leak nobody sees                     |
| `strings.Contains(err.Error(), ...)`                 | Breaks when the message changes; use `errors.Is` / `errors.As`         |
| `init()` that does I/O or reads configuration        | Runs on import, in tests, in tools, before `main` can handle the error |
| Package-level mutable state                          | Shared by every test and every goroutine, with no owner                |
| A `replace` directive in a released `go.mod`         | Ignored by consumers, so they build something different                |

---

## 9. What you produce

| Deliverable         | When                                               | What it looks like                                                        |
| ------------------- | -------------------------------------------------- | ------------------------------------------------------------------------- |
| ADR                 | Before any of the four decisions in §1             | `docs/decisions/NNNN-*.md`, Consequences and Alternatives filled          |
| Architecture review | On request, or before a package moves              | `severity · file:line · checklist row · fix`                              |
| C4 diagram          | Context and Container only                         | Mermaid in the repository; one container per binary                       |
| API design review   | Before an exported package or HTTP API has callers | Exported identifiers, error values, context use, versioning               |
| Refactoring plan    | To break a cycle or split a package                | Each step compiles and passes tests on its own; what moves, in what order |

## 10. How to run a review

1. `go list -m` and `go.mod`: the `go` and `toolchain` lines, `tool` entries,
   `replace` directives.
2. `go list ./...` — the package map. Every package outside `internal/` and
   `cmd/` is public API; ask whether it should be.
3. The gates in §3, in order. A red gate is the first finding.
4. golangci-lint with `containedctx` for contexts stored in structs;
   `grep -rn "go func" --include=*.go` and find the owner of each one.
5. Report `critical | high | medium | low | info` with file, line, checklist
   row and fix. A finding that cites no row is left out.

## 11. Where this expert stops

- **Security** — authentication, input handling, secrets, and what
  `govulncheck` reports: [`security-reviewer`](../security-reviewer/SKILL.md)
  and [`docs/review-standards.md`](../../docs/review-standards.md).
- **Build pipeline, images and deployment**: [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md).
- **Profiling and allocation tuning**: [`performance-engineer`](../performance-engineer/SKILL.md).
- **Formatting** — `gofmt` decides, and nobody argues with it.
