# References

Every rule in [`SKILL.md`](SKILL.md) and every checklist row rests on one of
these. A row that cites nothing is somebody's opinion and does not belong in an
audit ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

Each row carries the version current when it was last read, and the date of
that reading. **Re-check every 90 days.**

> **Next re-check due: 2026-12-23.**

WCAG 2.2, WCAG 3.0 and the ARIA Authoring Practices Guide are already tracked
in [`docs/review-standards.md`](../../docs/review-standards.md). They are listed
again below only to record that this expert re-read them on its own date; where
the two ever disagree, that file is the one to correct.

## The standard

| Reference                                                              | Version                                                                                                                                                    | Checked    | Used for                                                                                                                                                             |
| ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [WCAG 2.2](https://www.w3.org/TR/WCAG22/)                              | W3C Recommendation, first published 2023-10-05, current dated version 2024-12-12. 4.1.1 Parsing is obsolete and removed. Same row in `review-standards.md` | 2026-09-24 | Every success criterion cited in every checklist, and the Level AA target. The October 2023 text is also ISO/IEC 40500:2025                                          |
| [Understanding WCAG 2.2](https://www.w3.org/WAI/WCAG22/Understanding/) | companion to the 2024-12-12 version                                                                                                                        | 2026-09-24 | How each criterion is tested: the 320 CSS px reflow width (Z5), the text spacing values (Z6), the five target size exceptions (Z7), `scroll-padding` for 2.4.11 (K5) |
| [WCAG 3.0](https://www.w3.org/TR/wcag-3.0/)                            | Working Draft — **not** a standard. Tracked in `review-standards.md`                                                                                       | 2026-09-24 | Nothing. Recorded so nobody cites it as a requirement (SKILL.md §5)                                                                                                  |

## Semantics and widgets

| Reference                                                          | Version                                                                                      | Checked    | Used for                                                                                                                   |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------- |
| [WAI-ARIA 1.2](https://www.w3.org/TR/wai-aria-1.2/)                | W3C Recommendation 2023-06-06. WAI-ARIA 1.3 is a Working Draft (2026-06-04) and is not cited | 2026-09-24 | Roles, states and properties behind `screen-reader.md` SR3, and the refusal to cite ARIA 1.3                               |
| [ARIA in HTML](https://www.w3.org/TR/html-aria/)                   | W3C Recommendation 2026-08-11                                                                | 2026-09-24 | SR4 and the SKILL.md §5 refusal: a native element's implicit semantics are not to be duplicated or overridden with ARIA    |
| [ARIA Authoring Practices Guide](https://www.w3.org/WAI/ARIA/apg/) | undated, current at check. Same row in `review-standards.md`                                 | 2026-09-24 | `keyboard-and-focus.md` K8 and K9 — each widget's keyboard interaction table — and "no ARIA is better than bad ARIA" (SR4) |

## Method

| Reference                                 | Version                                                                                           | Checked    | Used for                                                                                                   |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------- |
| [WCAG-EM](https://www.w3.org/TR/WCAG-EM/) | **2.0**, W3C Group Note 2026-07-23. WCAG-EM 1.0 (web pages only) remains available; 2.0 adds apps | 2026-09-24 | The five steps in SKILL.md §2 — scope, explore, sample, evaluate, report — and `automated-baseline.md` AB3 |

## Tools

| Reference                                                                                              | Version                                                                                                                           | Checked    | Used for                                                                                                           |
| ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------ |
| [axe-core](https://github.com/dequelabs/axe-core)                                                      | 4.13.0, published 2026-08-05 (npm `latest`)                                                                                       | 2026-09-24 | AB1, AB7, and the README's statement that it finds on average about 57% of issues automatically (AB8, SKILL.md §5) |
| [@axe-core/cli](https://github.com/dequelabs/axe-core-npm/tree/develop/packages/cli)                   | 4.13.0, depends on `axe-core ~4.13.0`. It also depends on `chromedriver` at `latest`, so record the browser version in `scope.md` | 2026-09-24 | The command in AB1 (`--tags`, `--save`) and AB7 (`--exit` to fail a build)                                         |
| [axe API documentation](https://www.deque.com/axe/core-documentation/api-documentation/)               | current at check                                                                                                                  | 2026-09-24 | AB2 — tags are per WCAG version, so `wcag22aa` alone is a subset — and AB4, the meaning of `incomplete`            |
| [Lighthouse](https://github.com/GoogleChrome/lighthouse)                                               | 13.5.0, published 2026-09-18 (npm `latest`)                                                                                       | 2026-09-24 | AB5                                                                                                                |
| [Lighthouse accessibility scoring](https://developer.chrome.com/docs/lighthouse/accessibility/scoring) | current at check                                                                                                                  | 2026-09-24 | AB5 and SKILL.md §3: the score is a weighted average of axe-based audits, and manual audits do not affect it       |

Screen readers (NVDA, VoiceOver) are not pinned here. Their versions change
with the operating system, so each audit records the pair it used in
`scope.md` and in every `manual-log.md` row, and that record is what makes the
audit repeatable.

## Law and procurement — context, not advice

| Reference                                                                                                                                                                             | Version                                                                                                                                                                                                               | Checked    | Used for                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| [EN 301 549](https://www.etsi.org/deliver/etsi_en/301500_301599/301549/)                                                                                                              | **V4.1.1 (2026-09)**, published by ETSI 2026-09-02; moves the web, document and software clauses to WCAG 2.2. **V3.2.1 (2021-03)** stays the cited harmonised reference until V4.1.1 is cited in the Official Journal | 2026-09-24 | SKILL.md §7 — the European standard a compliance checklist is mapped to. State which version a report maps to; the two are not interchangeable       |
| [AccessibleEU — EN 301 549 has been updated](https://accessible-eu-centre.ec.europa.eu/content-corner/news/european-accessibility-standard-en-301-549-has-been-updated-2026-09-07_en) | news item dated 2026-09-07                                                                                                                                                                                            | 2026-09-24 | The source for the V3.2.1 / V4.1.1 legal-status distinction in the row above                                                                         |
| [European Accessibility Act, Directive (EU) 2019/882](https://eur-lex.europa.eu/eli/dir/2019/882/oj/eng)                                                                              | Directive of 17 April 2019 on the accessibility requirements for products and services; measures apply from 2025-06-28 (Art. 31), transitional period to 2030-06-28 (Art. 32)                                         | 2026-09-24 | SKILL.md §7 — why the question is being asked. The expert names the directive and the standard; whether a product falls under it is a legal question |

## Deferred to elsewhere

- Implementing fixes across a UI codebase: a frontend engineer expert, which
  the catalog has not written yet.
- Suite design — determinism, what to fake, CI signal:
  [`qa-automation-lead`](../qa-automation-lead/SKILL.md).
- The plain-language conformance statement and help pages:
  [`technical-writer`](../technical-writer/SKILL.md).
- Whether a product is legally compliant in a jurisdiction: a lawyer. This
  expert produces evidence against a standard, not a legal opinion.
