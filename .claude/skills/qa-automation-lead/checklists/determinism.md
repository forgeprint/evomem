# Determinism

A test that passes on retry has told you something is non-deterministic.
"Re-run the build" is the decision to stop listening.

| #    | Check                                                                      | How                                               | Source                                     |
| ---- | -------------------------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------ |
| DT1  | No `sleep` anywhere in the suite                                           | grep for it                                       | Wait for the condition, not the clock      |
| DT2  | No automatic retry configured                                              | read the runner and the CI configuration          | Retry hides the signal permanently         |
| DT3  | Time is injected, and frozen in tests that depend on it                    | grep for direct clock access                      | —                                          |
| DT4  | Randomness is seeded, and the seed is printed on failure                   | grep for unseeded generators                      | Reproducibility                            |
| DT5  | The suite passes when run in a shuffled order                              | run it with the runner's randomise option         | Order dependence                           |
| DT6  | The suite passes when run in parallel                                      | run it with workers; compare                      | Shared state                               |
| DT7  | Each test arranges everything it needs; nothing depends on a previous test | look for setup in one test that another relies on | —                                          |
| DT8  | Parallel workers do not share a database, a port or a directory            | read the fixture setup                            | Per-worker isolation                       |
| DT9  | No real network call outside the integration layer                         | run the unit suite with the network disabled      | —                                          |
| DT10 | Every quarantined test has an issue and an expiry date                     | read the quarantine list                          | A quarantine with no expiry is a graveyard |

## Why each one

**DT5 and DT6 are the two commands** that find most of this file's violations in
one run each, and almost nobody runs them. Shuffle the order and run in
parallel; anything that changes result was already broken and was passing by
coincidence.

**DT2** is the item that gets argued about. Retry does make the build greener,
and that is exactly the problem: the non-determinism is still there, it is now
invisible, and the first time it reflects a real race in the system nobody will
notice.

**DT4** matters on failure, not on success. Random data that finds a bug once
and cannot be reproduced is worse than no random data, because you now know
there is a bug and cannot get back to it.
