# References

Every rule in [`SKILL.md`](SKILL.md) and every checklist item rests on one of
these. An item that cites nothing is somebody's opinion and does not belong in
a review ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

Each row carries the version current when it was last read, and the date of
that reading. **Re-check every 90 days.**

> **Next re-check due: 2026-12-22.**

## The shape of a suite

| Reference                                                                                                     | Version | Checked    | Used for                                                                        |
| ------------------------------------------------------------------------------------------------------------- | ------- | ---------- | ------------------------------------------------------------------------------- |
| [Martin Fowler — Test Pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)                 | current | 2026-09-23 | The layer table in SKILL.md §4 — what each layer proves and what it must not do |
| [Martin Fowler — Mocks Aren't Stubs](https://martinfowler.com/articles/mocksArentStubs.html)                  | current | 2026-09-23 | `behaviour-not-implementation.md` B2, and the distinction the refusals rest on  |
| [Martin Fowler — Test Double](https://martinfowler.com/bliki/TestDouble.html)                                 | current | 2026-09-23 | `test-data.md` TD1, TD2 — which kind of double is appropriate where             |
| [Martin Fowler — Eradicating Non-Determinism in Tests](https://martinfowler.com/articles/nonDeterminism.html) | current | 2026-09-23 | All of `determinism.md`, and the quarantine policy in SKILL.md §2               |
| [Martin Fowler — Test Coverage](https://martinfowler.com/bliki/TestCoverage.html)                             | current | 2026-09-23 | `coverage.md` CV1 and CV2 — coverage as a finder of gaps, never as a target     |
| [Martin Fowler — Object Mother](https://martinfowler.com/bliki/ObjectMother.html)                             | current | 2026-09-23 | `test-data.md` TD4, TD5                                                         |
| [Martin Fowler — Continuous Integration](https://martinfowler.com/articles/continuousIntegration.html)        | current | 2026-09-23 | `ci-signal.md` CI2, CI4, CI8 — the gate, and what a permanently red build costs |
| [Martin Fowler — Semantic Conflict](https://martinfowler.com/bliki/SemanticConflict.html)                     | current | 2026-09-23 | `ci-signal.md` CI8 — why the merge result has to be built                       |

## Testing against real dependencies

| Reference                                     | Version          | Checked    | Used for                                                                                 |
| --------------------------------------------- | ---------------- | ---------- | ---------------------------------------------------------------------------------------- |
| [Testcontainers](https://testcontainers.com/) | current at check | 2026-09-23 | `test-data.md` TD3 — the real engine in a container, rather than an in-memory substitute |

## Measurement

| Reference                                                      | Version          | Checked    | Used for                                                         |
| -------------------------------------------------------------- | ---------------- | ---------- | ---------------------------------------------------------------- |
| [Mutation testing — Stryker](https://stryker-mutator.io/docs/) | current at check | 2026-09-23 | `coverage.md` CV5 — whether the assertions would notice a change |

## Deferred to elsewhere

- Real credentials, production data and anything else that must not enter a
  repository: CLAUDE.md §5b and
  [`security-reviewer`](../security-reviewer/SKILL.md).
- Pipeline permissions, pinned actions and what a build token can reach:
  [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md). This
  expert owns the signal; that one owns the pipeline.
- Whether the code under test is structured so it _can_ be tested — layering,
  dependency direction, composition root:
  [`dotnet-senior-architect`](../dotnet-senior-architect/SKILL.md) for .NET.
