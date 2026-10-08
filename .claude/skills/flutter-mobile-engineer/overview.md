# Flutter Senior Mobile Engineer

## What it changes

An agent asked to build a Flutter screen writes something that runs: a
`StatefulWidget` at the root with `setState` on every change, a
`Column` of mapped rows in a `SingleChildScrollView`, `jsonDecode` on the UI
isolate, `int.parse(state.pathParameters['id']!)`, a token in
`shared_preferences`, an `IconButton` with no tooltip, and a test that finds
everything by `Key`. With this expert:

- **The analyzer is strict and pinned.** `very_good_analysis` 11.0.0, or
  `flutter_lints` 6.0.0 plus the rules it leaves out — including
  `prefer_const_constructors`, which the Flutter performance page relies on and
  `flutter_lints` does not enable.
- **Rebuilds stay local**, lists are lazy, and work over a frame moves to
  `Isolate.run`.
- **One state approach, in an ADR.** Riverpod or Bloc — the research found the
  ecosystem split between them — with its own lint, and never both.
- **Routes parse their parameters**, auth is one `redirect`, and deep links do
  not act on arrival.
- **Secrets go to `flutter_secure_storage`** with Android backup excluded;
  `--dart-define` is treated as source.
- **Accessibility is a widget test.** The four `meetsGuideline` checks run in
  CI, and strings come from `gen-l10n`.
- **Goldens are compared on one pinned platform**, because fonts render
  differently elsewhere.

Six checklists — analysis gate, rebuild discipline, state and routing, device
storage, semantics and localisation, Flutter tests.

## What it fits

- Writing or reviewing screens, widgets, routes, storage and tests in a Flutter
  app on 3.47 stable, for Android and iOS.
- A pull request that adds a package, a platform channel, a route or a stored
  value.
- Setting up the gate a Flutter app keeps: lint set, analyzer modes, format
  check, guideline tests, golden platform.

## What it does not fit

- **Deciding the structure of a large Dart codebase** — package layering, pub
  workspaces, what is generated and committed, FFI boundaries. That is
  `dart-senior-architect`, proposed alongside this expert; this one builds
  inside the structure that one decides.
- **A React Native app.** Use `react-native-mobile-engineer`.
- **A native Android app in Kotlin.** Use `android-mobile-engineer`.
- **A native iOS app in Swift.** No sibling yet: a SwiftUI expert waits for a
  check that can run on macOS (expansion plan D21).
- **Flutter for web or desktop.** The storage, linking and accessibility rows
  assume Android and iOS.
- **A security audit** — MASVS depth, pinning, tamper resistance:
  [`security-reviewer`](../security-reviewer/SKILL.md).
- **A full accessibility audit**: the planned `accessibility-specialist`.

## Which mobile sibling

| The app is…                               | Use                            |
| ----------------------------------------- | ------------------------------ |
| TypeScript, React Native, usually on Expo | `react-native-mobile-engineer` |
| Dart and Flutter                          | `flutter-mobile-engineer`      |
| Kotlin and Jetpack Compose, Android only  | `android-mobile-engineer`      |

## Pros and cons

**In its favour:** most of its rows are enforced by tools Flutter already ships
— the analyzer, the guideline matchers, the golden comparator — so adopting it
is mostly configuration, and the result is a failing build rather than a
review comment.

**Against it:** it takes a side on some things teams argue about — widgets over
helper methods, one state library per app — and says so. It cannot run a
device or a screen reader. And it is `provenance: generated`: drafted by a tool
from the 2026-09-24 research, not written from someone's practice.

**What `agents: [claude-code]` rests on.** Claude Code ran the checklist greps
over this repository, which has no Dart code: they are well-formed and returned
nothing. It has not been run against a real Flutter app. If it changes nothing
about what your agent does on one, say so in an issue.
