# References

This expert deliberately holds **no standards of its own**. They live in
[`docs/review-standards.md`](../../docs/review-standards.md), which carries
each one with the version that was current when it was last read and the date
of that reading ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

Two copies of a standard is one copy that is wrong, and the wrong one will be
the copy somebody remembers.

> **The standards list's own re-check is due 2026-12-21.** If that date has
> passed, say so in the review: a stale list still produces a review, but the
> reader has to know.

## Which rows this expert uses

| Checklist               | Rows in `docs/review-standards.md`                                                                            |
| ----------------------- | ------------------------------------------------------------------------------------------------------------- |
| `trust-boundaries.md`   | OWASP ASVS (architecture), OWASP Top 10:2025 (A01), OWASP Top 10 for Agentic Applications                     |
| `authn-authz.md`        | OWASP ASVS (access control, session management, cryptography), OWASP API Security Top 10 (API1, API4, API5)   |
| `input-and-output.md`   | OWASP ASVS (injection, validation), OWASP API Security Top 10 (API3, API4), OWASP Top 10 for LLM Applications |
| `secrets-and-config.md` | OWASP Top 10:2025 (A02), ASP.NET Core security documentation, .NET secrets guidance, CIS Docker Benchmark     |
| `dependencies.md`       | OWASP Top 10:2025 (A03), NIST SSDF SP 800-218, SLSA v1.2, CIS Docker Benchmark                                |
| `logging-and-errors.md` | OWASP Top 10:2025 (A02, A10), OWASP ASVS (logging, authentication)                                            |

For an MCP server or an agent tool surface, add the MCP specification's
security best practices row and the NSA/CISA MCP guidance row.

## What this expert adds on top of the standards

Nothing about _what_ is dangerous — that is what the standards are for. What it
adds is how a review is conducted and written:

| Source                                                                        | Used for                                                                                                                                                   |
| ----------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLAUDE.md §5c and rule 23                                                     | A `critical` or `high` open finding blocks `tier: official`, which is what makes the boundary between `high` and `medium` a decision rather than a feeling |
| CLAUDE.md §5b                                                                 | The rules on what never enters a public repository, including that an example value is unmistakably fake or it is not an example                           |
| [`skills/blueprint-arch-review`](../../skills/blueprint-arch-review/SKILL.md) | The report format, and the two sections that make a report a review: **checked and found sound**, and **not applicable, with reasons**                     |

## How to cite

`ASVS 5.0 §<section>`, `API1:2023 BOLA`, `A03:2025 Software Supply Chain
Failures`, `SLSA v1.2 Build L<n>`, `CIS Docker <control>`. Always with the
version. **If you cannot find the control number, describe the weakness without
one** — a guessed citation is a wrong answer wearing a uniform, and it is worse
than an honest sentence.
