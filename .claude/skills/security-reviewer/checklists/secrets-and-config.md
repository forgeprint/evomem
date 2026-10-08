# Secrets and configuration

Security misconfiguration rose to second in the 2025 Top 10. Most of what is
found here is not clever — it is a default nobody changed.

| #   | Check                                                                         | How                                                                                  | Control                        |
| --- | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | ------------------------------ |
| S1  | No secret in the repository, in any commit, including as an example           | scan history, not only the tip                                                       | A02:2025; §5b                  |
| S2  | An example value is unmistakably fake                                         | read them: a plausible-looking fake is worse than a real one, because it gets copied | §5b                            |
| S3  | Secrets arrive from the environment or a secret store, never a committed file | read the configuration sources in order                                              | Platform secrets guidance      |
| S4  | A missing secret fails startup loudly; no secret has a default                | remove one and start the app                                                         | A02:2025                       |
| S5  | No secret reaches an image layer, a build argument or a log line              | read the Dockerfile and the CI job                                                   | CIS Docker Benchmark; A02:2025 |
| S6  | Debug, verbose errors and developer endpoints are off outside development     | find the environment switch                                                          | A02:2025                       |
| S7  | CORS names origins; no wildcard with credentials                              | read the CORS policy                                                                 | A02:2025                       |
| S8  | TLS everywhere it leaves the process; no plain `http` to a real host          | grep the URLs in configuration and code                                              | A02:2025                       |
| S9  | The container runs as a non-root user, from a pinned base image               | read the Dockerfile                                                                  | CIS Docker Benchmark           |

## Why each one

**S2** is the rule people argue with. The argument is settled by what happens
next: a realistic-looking example is pasted into a real configuration file
because it looked like the shape of the thing, and now a real system has a
value nobody chose. `sk-EXAMPLE-0000` cannot have that outcome.

**S4** fails in the direction people find surprising: the danger is not that
the app crashes. It is that it starts, in a configuration nobody intended, and
says nothing.

**S7** is specific on purpose. A wildcard origin is a bad idea; a wildcard
origin _with credentials_ is a cross-origin read of authenticated data, which
the browser will refuse — until somebody works around the refusal.
