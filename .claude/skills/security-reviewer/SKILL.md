---
name: security-reviewer
description: Review a codebase or a design as a security surface — trust boundaries first, then authorization, input, secrets, dependencies and what the logs give away. Every finding cites a named control at a checked version, carries a severity, and says how to fix it. Use before shipping, before a boundary changes, when asked whether something is safe to build on, or when a review has to be written down rather than felt.
license: CC-BY-4.0
---

# Reviewing something as a security surface

A security review that produces a list of worries is not a review. It is
anxiety with headings. What makes it a review is that every item names a
control, a place, a severity and a fix — and that the reviewer is equally
willing to write down what they checked and found sound.

This skill is the procedure and the discipline. The **standards** live in
[`references.md`](references.md), which points at
[`docs/review-standards.md`](../../docs/review-standards.md) rather than
restating it; a standard copied twice is a standard wrong once.

---

## 1. The rule that makes the rest work

> **Every finding cites a control. A finding that cites nothing is an opinion,
> and it does not go in the report.**

Not "OWASP" — the control. `ASVS 5.0 §2.1.x`, `API1:2023 BOLA`,
`A03:2025 Software Supply Chain Failures`. With the version, because a control
cited at the wrong version sounds authoritative and is wrong.

If you believe something is dangerous and no control covers it, that is still
worth saying — say it under **Observations**, explicitly marked as not resting
on a standard. The reader can then weigh it as judgement rather than mistaking
it for a requirement.

---

## 2. Start at the trust boundaries, not at the code

An agent asked to review security will start grepping for `eval` and hardcoded
passwords. That finds the easy things and misses the architecture. Do this
instead, in order:

1. **Draw the boundaries.** Where does data cross from something you do not
   control to something you do? Every one of these: the network edge, the
   database, the message broker, every third-party API, the filesystem, the
   agent tool surface, the CI runner.
2. **For each boundary, name what crosses it** — and in which direction. Most
   real findings are about the direction nobody thought about: what leaves,
   not what arrives.
3. **For each boundary, ask the four questions:**
   - Who is the caller, and how do we know? (authentication)
   - What may _this_ caller do to _this_ object? (authorization)
   - What is the shape of what crosses, and who enforces it? (validation)
   - What does the failure look like, and what does it tell the caller?
4. **Only then read the code**, and read it in the order the data flows.

The single most common real vulnerability in an API is not injection. It is
**broken object-level authorization**: the endpoint checks that you are logged
in and never checks that the row is yours. It is invisible to a scanner and it
is visible in step 3.

See [`checklists/trust-boundaries.md`](checklists/trust-boundaries.md).

---

## 3. Severity means something specific

Use exactly five, and apply them consistently or the report is noise:

| Severity   | Means                                                                              | Example                                      |
| ---------- | ---------------------------------------------------------------------------------- | -------------------------------------------- |
| `critical` | Exploitable now, by an unauthenticated caller, with material impact                | Auth bypass; a secret in a public repository |
| `high`     | Exploitable by an authenticated caller, or by an attacker who has one step already | BOLA on a tenant-scoped resource             |
| `medium`   | Requires unusual conditions, or the impact is contained                            | Missing rate limit on an expensive endpoint  |
| `low`      | Defence in depth; no path to impact on its own                                     | A missing security header                    |
| `info`     | Not a weakness. Worth knowing                                                      | A dependency one major version behind        |

**A `critical` or `high` finding blocks** — in this catalog, it blocks
`tier: official` (rule 23). That is what makes the boundary between `high` and
`medium` a real decision rather than a feeling, so state the _path to impact_
when you claim either. If you cannot describe the path in one sentence, it is
not `high`.

---

## 4. What you check, in order

Each of these has a checklist; run the ones the thing under review actually
claims. Reviewing a static site against the database checklist produces
findings that are not findings.

1. **Trust boundaries** — the step above. Everything else depends on it.
2. **[Authentication and authorization](checklists/authn-authz.md)** — who the
   caller is, and the object-level check. This is where the real ones are.
3. **[Input and output](checklists/input-and-output.md)** — parameterised
   queries, encoding at the sink, deserialisation, file upload, SSRF.
4. **[Secrets and configuration](checklists/secrets-and-config.md)** — where
   secrets live, what happens when one is missing, what ships in the image.
5. **[Dependencies and the build](checklists/dependencies.md)** — pinning,
   provenance, what the CI token can do. Supply chain is now its own Top 10
   category and it is the one most often skipped.
6. **[Logging and errors](checklists/logging-and-errors.md)** — what the logs
   give away, and what the error message tells an attacker.

If the thing under review ships MCP tools or drives an LLM, add the tool
surface: what each tool can reach, whether its arguments are validated, and
whether the model's output is ever treated as a command. `docs/review-standards.md`
carries the OWASP LLM and Agentic references for this.

---

## 5. What you do not report

A review that reports everything is a review nobody reads twice. Leave out:

- **Findings the framework already prevents.** ORM parameterisation, the
  framework's own XSS encoding, its CSRF token. Say so once under "checked and
  sound" rather than as a finding per file.
- **Style.** Naming, formatting, structure. Real, and not this.
- **"Consider using X".** If X is not required by a control, it is advice, and
  advice goes under Observations or nowhere.
- **A missing control on something that does not exist.** No user uploads means
  no upload findings. The manifest or the design says what is claimed; review
  against that, and note explicitly what is _not applicable and why_.

The "not applicable, with reasons" section is not filler. It is how the reader
tells a review that considered something and dismissed it from a review that
never looked.

---

## 6. What you produce

| Deliverable          | What it is                                                                                                                                                                            |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Security review      | Findings by severity, each with `file:line`, the control, the path to impact, the fix. Plus **checked and sound** and **not applicable, with reasons**. Verdict: `MERGE` or `CHANGES` |
| Threat model         | Boundaries, what crosses each, and the threats per boundary. One page, not a document nobody opens                                                                                    |
| Threat model review  | The same, applied to somebody else's — including what their model leaves out                                                                                                          |
| Dependency audit     | Direct and transitive, with the pinning story and what the build token can reach                                                                                                      |
| Compliance checklist | The controls that apply, each marked met, not met, or not applicable, with evidence                                                                                                   |

A report is committed, not delivered verbally. In this repository that is
`docs/reviews/<slug>/<YYYY-MM-DD>.md` (§5c). A review nobody can re-read is a
review that will be argued about from memory.

---

## 7. How you behave when you are unsure

- **Do not guess a CVE number, a control number or a version.** Look it up, or
  describe the weakness without the number. A wrong citation is worse than
  none — it is a wrong answer wearing a uniform.
- **Do not claim exploitability you have not reasoned through.** "Could be
  exploitable" belongs in Observations, not in a `high`.
- **Never write an exploit** into a review. Describe the path; the fix is the
  deliverable.
- **Do not touch production data, a live system or somebody's credentials** to
  confirm a finding. A review reads; it does not test against things it was not
  given.
