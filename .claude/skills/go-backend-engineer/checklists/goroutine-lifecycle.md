# Goroutine lifecycle

A goroutine is cheap to start and has no owner unless somebody writes one.
Every row here answers the same question for a different goroutine: who stops
it, and who waits for it to stop?

| #    | Check                                                                                               | How                                                                          | Source                                                                        |
| ---- | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| GL1  | No handler starts a goroutine that outlives the request unless it hands work to an owned worker     | grep `go func` and `go ` calls inside handlers                               | Go 1.27 — Effective Go, goroutines; Gin 1.12 — goroutines inside a middleware |
| GL2  | Fan-out uses `errgroup.WithContext` or `sync.WaitGroup.Go`, and the caller waits                    | read every place that starts more than one goroutine                         | golang.org/x/sync 0.23 — errgroup; Go 1.25 — sync.WaitGroup.Go                |
| GL3  | Every long-running goroutine selects on `ctx.Done()`                                                | read each worker loop                                                        | Go 1.27 — package context                                                     |
| GL4  | `main` derives its context from `signal.NotifyContext` for `SIGINT` and `SIGTERM`                   | read `main.go`                                                               | Go 1.27 — os/signal NotifyContext                                             |
| GL5  | Shutdown fails readiness, calls `srv.Shutdown` with a deadline, stops workers, then closes the pool | send SIGTERM during a slow request; it completes, then the process exits 0   | Go 1.27 — net/http Server.Shutdown; Kubernetes 1.37 — pod termination         |
| GL6  | `ListenAndServe` returning `http.ErrServerClosed` is treated as a normal exit                       | read the error handling around `ListenAndServe`                              | Go 1.27 — net/http Server.Shutdown                                            |
| GL7  | `/healthz` checks only the process; `/readyz` pings the pool                                        | stop the database; `/healthz` stays 200, `/readyz` turns 503                 | Kubernetes 1.37 — container probes                                            |
| GL8  | Retries are bounded, use exponential backoff with jitter, and stop when the context is done         | read the retry helper; it waits on a timer inside `select` with `ctx.Done()` | RFC 9110 §9.2.2; Go 1.27 — math/rand/v2                                       |
| GL9  | Every test package runs goleak in `TestMain`                                                        | grep `goleak.VerifyTestMain`                                                 | goleak 1.3.0                                                                  |
| GL10 | The service exposes pprof on an internal port, including Go 1.27's `goroutineleak` profile          | read the debug server; it is not on the public listener                      | Go 1.27 release notes — goroutine leak profile                                |

## Why each one

**GL1** is the leak that also races. A goroutine started in a handler and never
waited for keeps running after the response is sent; with Gin it may be holding
a `*gin.Context` that is now serving somebody else's request.

**GL5** has to be observed, not read. `Shutdown` stops new connections and waits
for active ones, but only if the orchestrator's grace period is longer than the
deadline passed to it, and only if readiness failed first so no new traffic
arrives during the drain.

**GL9 and GL10** are the same check at two points in time. goleak fails the
test run if anything is still running when the tests finish; the
`goroutineleak` profile, generally available in Go 1.27, finds goroutines
blocked forever in a live process.
