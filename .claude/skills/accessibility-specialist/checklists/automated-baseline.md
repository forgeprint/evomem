# Automated baseline

Automation runs first because it is cheap and it removes the obvious, so the
manual passes can spend their time on what only a person finds. It is the
floor of an audit, never the audit.

| #   | Check                                                                                                 | How                                                                                                                                                     | Source                                  |
| --- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| AB1 | axe-core runs at a pinned version, and the version is recorded                                        | `npx @axe-core/cli@4.13.0 <url> --tags wcag2a,wcag2aa,wcag21a,wcag21aa,wcag22aa --save <page>.json`; read `testEngine.version` in the saved file        | axe-core 4.13.0; axe API documentation  |
| AB2 | The tag set covers every WCAG version up to 2.2, not only the rules added in 2.2                      | read the command or the test configuration; `wcag22aa` alone is the finding                                                                             | axe API documentation — tags            |
| AB3 | Every sampled page and every state in an essential process has its own result file                    | compare `sample.md` with the files in `axe/`; an open dialog and an error state are states                                                              | WCAG-EM 2, step 4                       |
| AB4 | Every `incomplete` item is carried into the manual log with a human verdict                           | count `incomplete` entries in the JSON; each one appears in `manual-log.md`                                                                             | axe API documentation — `incomplete`    |
| AB5 | Lighthouse's accessibility category is run as a second opinion, pinned, and not reported as a verdict | `npx lighthouse@13.5.0 <url> --only-categories=accessibility --output=json --output-path=<page>.json`; the report never states the score as conformance | Lighthouse accessibility scoring        |
| AB6 | Each violation is reported with the success criterion it maps to, not the rule id alone               | a rule id such as `color-contrast` becomes `1.4.3 Contrast (Minimum)` in the report                                                                     | WCAG 2.2; axe-core 4.13.0               |
| AB7 | The same pinned rules run in the project's own end-to-end suite after the audit                       | find the test that calls axe; break a label on purpose and watch it fail                                                                                | axe-core 4.13.0; @axe-core/cli `--exit` |
| AB8 | A clean automated run is recorded as "no automated violations", never as "accessible"                 | read the summary line of the report                                                                                                                     | axe-core README — detection is partial  |

## Why each one

**AB2 is the mistake that looks like diligence.** axe tags rules by the WCAG
version that introduced them, so a run with `wcag22aa` alone checks the
handful of rules new in 2.2 and skips contrast, names and labels entirely. The
report then says "WCAG 2.2 AA: no violations" about a page nobody tested.

**AB4** is where automation hands over. `incomplete` means the engine could not
decide — contrast over a background image, text that may be hidden. Dropping
those is the same as marking them passed.

**AB8** is the sentence people want to write and must not. axe-core's own
README says it finds on average about 57% of issues automatically; the
remainder is the rest of this expert.
