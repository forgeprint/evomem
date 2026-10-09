# QA Automation Lead

## What it changes

An agent asked to add tests adds tests. They pass, coverage goes up, and the
suite is now slightly harder to refactor around and no better at catching
anything. Four things change with this expert:

- **Tests are named as behaviours and assert on outcomes.** The direct check is
  the one nobody runs: rename a class, extract a method, and the suite must
  stay green. A suite that breaks on a behaviour-preserving refactor is coupled
  to the code, not to what the code is for.
- **A flaky test is a failed test.** Quarantined the day it is noticed, with an
  issue and an expiry; fixed or deleted. Automatic retry is a finding, not a
  fix — it makes the build greener by making the signal permanently invisible.
- **Coverage is never a target.** It is used to find what is untested, and then
  somebody decides whether it matters. The measurement that actually answers
  the question is breaking something on purpose and seeing whether the suite
  notices.
- **The signal has to be actionable.** A gate people skip is not a gate, and a
  build that is red half the time gates nothing.

Five checklists — behaviour not implementation, determinism, coverage, test
data and doubles, CI signal — and eleven refusals, from `sleep` to a test named
after a ticket number.

## What it fits

- Writing a new suite, or adding tests to a codebase that has few.
- Reviewing an existing suite, where the procedure starts by running it twice
  in different orders and ends by deliberately breaking something.
- Diagnosing a flaky pipeline, which is the problem this expert has the most to
  say about.
- Deciding what to fake and what to test against for real — the decision that
  determines what the suite can ever catch.

## What it does not fit

- **Manual and exploratory testing.** Everything here is about an automated
  suite. Exploratory testing is a genuine discipline and a different one.
- **Performance and load testing.** It will refuse a `sleep` and stop there.
- **Security testing.** Belongs with
  [`security-reviewer`](../security-reviewer/SKILL.md).
- **Accessibility testing.** A specialism with its own criteria; the taxonomy
  has a role for it and this is not that role.
- **Pipeline configuration.** Permissions, pinned actions and what the build
  token can reach belong to
  [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md). This
  expert owns the signal; that one owns the pipeline.
- **A codebase that cannot be tested yet.** If there is no seam to test
  against, the finding is architectural and it goes to an architect first.

## Pros and cons

**In its favour:** almost every item is a command rather than an argument. Run
the suite shuffled, run it in parallel, time it, delete a validation rule and
see what happens. Those four take an afternoon and tell you more about a suite
than reading it for a week.

**Against it:** the flaky-test policy is the part teams negotiate away, and this
expert does not offer a middle position — it treats retry as a finding, which
will be unwelcome in a pipeline that currently depends on it. It is also
language-agnostic by design, so it names no runner and no assertion library,
and somebody has to translate each check into their own tooling.
