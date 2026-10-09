# Accessibility Specialist

## What it changes

An agent asked to "check accessibility" runs a scanner, fixes what it reports,
and says the page is accessible. The most widely used engine's own README puts
what it finds automatically at about 57% of issues on average. Four things change with this expert:

- **Automation is the floor.** axe-core runs pinned, with the full WCAG 2.2 AA
  tag set, on every sampled page and state, and every `incomplete` result goes
  to a person. A clean run is recorded as "no automated violations", never as
  "accessible".
- **The manual passes are the audit.** Keyboard only, a named screen reader and
  browser pair, 200% and 400% zoom, contrast including focus indicators and
  borders, target size, forms submitted empty and wrong, motion with the
  reduced-motion setting on and off.
- **Every finding cites a success criterion by number and gives a
  reproduction.** `2.4.11 Focus Not Obscured (Minimum)`, the page, the
  element, the steps, the assistive technology. A finding without both is an
  opinion.
- **Each step leaves a file.** Scope with an accessibility support baseline,
  the sample, the axe and Lighthouse JSON, a manual log with an SC number on
  every row, and the report — so somebody else can check the audit happened.

Six checklists — automated baseline, keyboard and focus, screen reader, zoom
contrast and targets, forms and errors, motion and media — and eleven
refusals, from "passes axe" as a conformance claim to ARIA where a native
element works.

## What it fits

- Auditing a web interface or an app against WCAG 2.2 Level AA, with a sample
  chosen the way WCAG-EM 2 describes.
- Reviewing a component or a pull request that touches UI: native element
  first, name, role and state, focus handling.
- Wiring the automated rules into a project's own end-to-end suite after an
  audit, so a regression fails a build.
- Preparing the evidence behind a conformance report — every A and AA
  criterion marked supports, partially supports, does not support or not
  applicable.
- A product with European customers that needs to know which standard applies:
  it names EN 301 549 and the European Accessibility Act, and which version of
  the standard is cited in law today.

## What it does not fit

- **Implementing fixes across a UI codebase.** It names the criterion, the
  element and the fix, and writes a change when asked. Rebuilding a component
  library belongs to a frontend engineer — an expert this catalog has not
  written yet.
- **Visual design.** It measures contrast, spacing and target size against
  numbers. Whether the result looks good is not its question.
- **Legal advice.** It points at WCAG 2.2, EN 301 549 and Directive (EU)
  2019/882 and says what the evidence shows. Whether a product falls under a
  law, or complies with it, is for a lawyer. It will not write "compliant".
- **Certification.** It is not an accredited auditor, and its report is not a
  certificate.
- **Testing with disabled users.** An audit against criteria finds failures of
  the standard. It does not replace usability testing with people who use
  assistive technology every day, and it says so in its report.
- **Native desktop software, documents and hardware** at the depth EN 301 549
  covers them. The success criteria carry over; the procedure and the tools
  here are written for web and app interfaces.

## Pros and cons

**In its favour:** it is built around the gap scanners leave. Every step is a
file somebody can open, every finding carries a number a developer can look
up, and the refusals catch the three claims that do the most damage — "passes
axe", "Lighthouse 100" and "compliant".

**Against it:** a real audit takes time, and this expert does not offer a
shortcut. The screen reader pass needs a person at a machine with NVDA or
VoiceOver; an agent cannot hear the output, so on its own it can prepare the
log and run the automated half, and must leave the manual rows marked
unchecked rather than guess. It is also `provenance: generated` — drafted from
research rather than from somebody's practice — and it should be read with
that in mind until an accessibility practitioner has used it and said what it
missed.
