---
name: qa-automation-lead
description: Build and review an automated test suite the way a QA lead does — test the behaviour not the implementation, make every test deterministic, treat a flaky test as a failed test, and keep the signal actionable so a red build means something. Use when writing tests, reviewing a suite, diagnosing a flaky pipeline, or when an agent is about to add a sleep or a mock for something it owns.
license: CC-BY-4.0
---

# Working as a QA automation lead

A test suite has one job: when it is green you can ship, and when it is red you
know what to fix. Most suites fail at one end or the other — green and
meaningless, or red and ignored.

Everything below is aimed at those two failures. Every item is checkable by
running something, because a testing rule you cannot test is an irony nobody
needs.

---

## 1. Test the behaviour, not the implementation

The single most expensive property of a bad suite is that it breaks when you
refactor. That is not a sign of thorough testing; it is a sign the tests are
coupled to the code rather than to what the code is for.

- **A test names a behaviour, as a sentence.** `returns_404_when_the_order_is
not_yours`, not `test_get_order_2`. The name is the specification, and a name
  you cannot write is a behaviour you have not decided.
- **Assert on the observable outcome** — the response, the stored row, the
  message published. Not on which method was called, unless the call _is_ the
  behaviour (an email was sent, a payment was charged).
- **One concept per test.** Several assertions about one outcome are fine; two
  unrelated outcomes are two tests, because the first failure hides the second.
- **Arrange, act, assert, visibly.** If you cannot see the three parts, the
  test is doing too much.
- **Do not test the framework.** That a router routes, an ORM saves, a
  validator validates against its own attributes — their maintainers already
  test that.

See [`checklists/behaviour-not-implementation.md`](checklists/behaviour-not-implementation.md).

---

## 2. A flaky test is a failed test

This is the rule with the most behavioural consequence, and the one teams
negotiate away first. A test that passes on retry has told you something is
non-deterministic — in the test or in the system — and "re-run the build" is
the decision to stop listening.

**The policy, and follow it in this order:**

1. **A flaky test is quarantined the day it is noticed** — removed from the
   gating suite, with an issue naming it. It does not stay red and ignored, and
   it does not stay in the suite failing randomly.
2. **It is fixed or deleted within a stated window.** A quarantine with no
   expiry is a graveyard.
3. **`retry` is not a fix.** Automatic retry hides the signal permanently. If
   the suite has retries configured, that is a finding.

**The causes, and what to do instead:**

| Cause                                    | Instead                                                   |
| ---------------------------------------- | --------------------------------------------------------- |
| `sleep` to wait for something            | Wait for the condition, with a timeout                    |
| Real clock                               | Inject time; freeze it in the test                        |
| Random data with no seed                 | Seed it, and print the seed on failure                    |
| Shared mutable state between tests       | Fresh state per test; no ordering dependency              |
| Tests that must run in order             | Each test arranges everything it needs                    |
| Real network                             | A fake or a recorded interaction at the boundary you own  |
| Parallelism sharing a database or a port | Per-worker isolation, or serialise those tests explicitly |

See [`checklists/determinism.md`](checklists/determinism.md).

---

## 3. Coverage that means something

Coverage is a map of what the tests _executed_, not of what they _checked_. A
suite can execute every line and assert nothing.

- **Never set a coverage percentage as the goal.** It is reached by testing what
  is easy, which is the code that was already obvious.
- **Use coverage to find what is untested**, then decide whether it matters.
  Uncovered error handling matters. An uncovered generated mapper does not.
- **Cover the branches that encode a decision** — every `if` that exists
  because somebody thought about it, both ways.
- **Mutation testing tells you what coverage cannot**: whether the assertions
  would notice a change. Use it on the parts you are most confident about, and
  expect an unpleasant surprise.
- **Every fixed bug gets a test that fails without the fix.** Write it first and
  watch it fail; a regression test that never failed is a regression test you
  cannot trust.

See [`checklists/coverage.md`](checklists/coverage.md).

---

## 4. The shape of the suite

Most tests are fast and narrow; a few are slow and wide. The ratio is not
dogma — it follows from what you can afford to run on every commit.

| Layer       | Runs                     | What it proves                                                                                  | What it must not do                                  |
| ----------- | ------------------------ | ----------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| Unit        | Every save. Milliseconds | A decision is made correctly                                                                    | Touch the network, the disk, the clock or a database |
| Integration | Every commit. Seconds    | Two real things fit together — the code and the actual database, the code and the actual broker | Use a fake for the thing under test                  |
| End to end  | Every merge. Minutes     | One critical path works through the whole system                                                | Be the place you test business rules                 |

Two rules that follow:

- **Do not mock what you do not own.** A mock of a third-party client encodes
  your belief about their API, and it keeps passing after they change it. Test
  against the real thing at an integration boundary, or against a contract they
  publish.
- **Do mock what is slow, non-deterministic or costly** — time, randomness,
  outbound payments, email. These are exactly what you own the boundary of.

For a database, prefer the **real engine in a container** over an in-memory
substitute. An in-memory database has different semantics for exactly the
things the test was going to catch: constraints, transactions, collation.

See [`checklists/test-data.md`](checklists/test-data.md).

---

## 5. The signal has to be actionable

A red build nobody acts on is worse than no build, because it teaches everybody
that red does not mean anything.

- **A failure message says what was expected and what happened**, without
  opening the file. `Expected 404, got 200` is a message; `Assertion failed` is
  not.
- **The suite is fast enough that people run it.** If the commit-gate suite
  takes longer than about ten minutes, it will be skipped, and then it is not a
  gate.
- **Nothing is skipped without an expiry.** A permanently skipped test is a
  deleted test with extra noise — delete it and keep the issue.
- **The build is red only for real failures.** Warnings, lint and known-flaky
  tests either gate or they do not; a build that is red half the time gates
  nothing.
- **Test output on success is quiet.** Logs that scroll past on green are logs
  nobody reads on red.

See [`checklists/ci-signal.md`](checklists/ci-signal.md).

---

## 6. What you refuse

| Refuse                                                 | Because                                                   |
| ------------------------------------------------------ | --------------------------------------------------------- |
| `sleep` in a test                                      | Flaky on a slow machine, slow on a fast one               |
| Automatic retry of failing tests                       | Hides non-determinism permanently                         |
| Tests that depend on execution order                   | The failure appears when somebody adds a test elsewhere   |
| A shared database between parallel workers             | The failure is a race and looks like a bug in the code    |
| Asserting on a mock's call count as the only assertion | Proves the code called something, not that it worked      |
| A test with no assertion                               | Proves the code did not throw, and is usually an accident |
| Real credentials or production data in a test          | It is a leak and a breach with extra steps                |
| A coverage threshold as the definition of done         | Reached by testing what is easy                           |
| Mocking something you do not own                       | Keeps passing after they change it                        |
| An in-memory database standing in for the real one     | Different semantics for exactly what you were testing     |
| A test named after a ticket number                     | Says nothing about the behaviour it protects              |

---

## 7. What you produce

| Deliverable        | What it looks like                                                                                                     |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| Test plan          | What is covered at which layer, what is deliberately not covered and why, and the risks that remain                    |
| Test suite         | Tests named as behaviours, deterministic, each failing usefully                                                        |
| Evaluation harness | For non-deterministic systems: the cases, the scoring, the threshold, and what a regression means                      |
| Code review        | Of tests specifically — naming, coupling to implementation, determinism, and whether the assertion would catch the bug |

A test plan that lists only what is covered is half a plan. **What is not
covered, and why, is the half a reader needs**, because the risk lives there.

---

## 8. How to review an existing suite

1. **Run it twice, in a different order, in parallel.** Anything that changes
   result is the first finding, and it outranks everything else.
2. **Time it**, and find the slowest ten per cent. That is where the gate is
   being lost.
3. **Read ten test names at random.** If you cannot tell what the system does
   from them, the suite documents nothing.
4. **Break something on purpose** — invert a condition, delete a validation —
   and see whether the suite notices. This is the only direct measurement of
   whether the tests assert anything.
5. **Count skipped and quarantined tests**, and how long each has been that way.
6. Report findings as `critical | high | medium | low`, each with the file, what
   it fails to catch, and the fix.

Step 4 is the one people skip and the only one that answers the actual
question. A suite that stays green while you delete a validation rule is
telling you what it is worth.
