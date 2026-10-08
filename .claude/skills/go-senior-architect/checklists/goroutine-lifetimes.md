# Goroutine lifetimes

Starting a goroutine costs one keyword. Knowing when it ends is the design
work, and a goroutine with no answer to that is a leak waiting for traffic.

| #   | Check                                                                                   | How                                                            | Source                                                   |
| --- | --------------------------------------------------------------------------------------- | -------------------------------------------------------------- | -------------------------------------------------------- |
| GL1 | Every `go` statement has an owner that waits for it                                     | `grep -rn "go func\|go [a-z]" --include=*.go`; find the `Wait` | Go Code Review Comments — goroutine lifetimes            |
| GL2 | Fan-out uses `errgroup.WithContext` or `sync.WaitGroup.Go`                              | read the concurrent sections                                   | golang.org/x/sync 0.23 — errgroup; Go 1.25 release notes |
| GL3 | Every long-running loop selects on `ctx.Done()`                                         | read each `for {` inside a goroutine                           | `context` package documentation                          |
| GL4 | A channel is closed by its sender, exactly once                                         | read each `close(`; no close on the receiving side             | Go spec — close; Effective Go — channels                 |
| GL5 | The HTTP server stops with `Shutdown(ctx)` on SIGTERM, with a deadline                  | read `main`; `signal.NotifyContext` and `srv.Shutdown`         | `net/http` — Server.Shutdown; `os/signal`                |
| GL6 | Tests run with `-race`, and CI fails on a race report                                   | the CI script                                                  | Go — Data race detector                                  |
| GL7 | A running service exposes the `goroutineleak` profile to operators, not to the internet | `net/http/pprof` on an internal listener only                  | Go 1.27 release notes — goroutine leak profile           |
| GL8 | No package-level mutable variable is written from more than one goroutine               | golangci-lint `gochecknoglobals`; read each exception          | Google Go style — global state                           |
| GL9 | Concurrency-sensitive tests use `testing/synctest`, not `time.Sleep`                    | `grep -rn "time.Sleep" --include=*_test.go`                    | `testing/synctest` documentation                         |

## Why each one

**GL1** is the rule from the Go code review guidance: when you spawn a
goroutine, make it clear when — or whether — it exits. A goroutine blocked on
a channel nobody will send to keeps its stack and everything it references
alive for the life of the process.

**GL5** because a service that exits on SIGTERM without `Shutdown` drops every
in-flight request on each deploy, and the error rate graph shows a spike at
every release that nobody can explain.

**GL7** turns leak-hunting from inference into a reading. Go 1.27 made the
`goroutineleak` profile generally available: it reports goroutines blocked on
primitives no running code can reach. It belongs behind an internal listener
because pprof output describes the program to whoever can fetch it.
