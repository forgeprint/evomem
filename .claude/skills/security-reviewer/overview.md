# Security Reviewer

## What it changes

An agent asked to "check this for security" greps for hardcoded passwords and
`eval`, finds two style issues, and reports them next to a real problem with
the same weight. Three things change with this expert:

- **It starts at the trust boundaries, not at the code.** Boundaries first,
  what crosses each one in both directions, then the four questions per
  boundary. Only then does it read code, and it reads it in the order the data
  flows.
- **Every finding cites a control at a version** — `ASVS 5.0 §2.1.x`,
  `API1:2023 BOLA`, `A03:2025` — or it is not a finding. Anything the reviewer
  believes but cannot cite goes under Observations, marked as judgement.
- **Severity is defined, and `high` requires a path to impact you can state in
  one sentence.** In this catalog a `critical` or `high` blocks `tier: official`,
  so the line between `high` and `medium` is a decision somebody is accountable
  for.

Six checklists: trust boundaries, authentication and authorization, input and
output, secrets and configuration, dependencies and the build, logging and
errors.

## What it fits

- A review that has to be **written down** rather than felt, and re-read later.
- An API, where three of the top five risks are authorization failures and the
  most common single vulnerability is object-level.
- A service that ships MCP tools or drives a model — the tool surface is a
  trust boundary and it is the least reviewed one.
- Deciding whether something is safe to build on, which is a different question
  from whether it has bugs.

## What it does not fit

- **Penetration testing.** It reads; it does not probe. It will not test
  against a live system, touch production data, or use credentials it was not
  given.
- **Writing exploits.** It describes the path to impact. The fix is the
  deliverable.
- **Compliance certification.** It can produce a compliance checklist against
  controls that apply; it is not an auditor and says so.
- **Architecture.** Layering, dependency direction and transaction boundaries
  belong to an architect — in this catalog,
  [`dotnet-senior-architect`](../dotnet-senior-architect/SKILL.md) for .NET.
  Pair them; do not ask one to do the other's half.
- **Anything with no trust boundary.** A pure library with no I/O has almost
  nothing here, and the honest output is a short "not applicable" list.

## Pros and cons

**In its favour:** it is the only expert here that is deliberately empty of
standards. Everything it cites lives in `docs/review-standards.md` with a
version and a date, so the review ages with the standards rather than with the
skill — and when the list is re-checked, every review written afterwards is
current without this file changing.

**Against it:** that indirection is a real cost. The expert is useless outside a
repository that keeps a standards list, and its value degrades exactly as fast
as that list goes stale. It also refuses to be quick: the boundary-first
procedure takes longer than a grep, and on a small change that is overhead
somebody has to be willing to pay.
