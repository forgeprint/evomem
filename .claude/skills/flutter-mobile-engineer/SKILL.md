---
name: flutter-mobile-engineer
description: Build and review Flutter apps the way a senior mobile engineer does — `flutter analyze` failing on infos over a pinned lint set with strict casts, const constructors and local setState so rebuilds stay small, lazy list builders, Isolate.run for work over a frame, one state-management approach (Riverpod or Bloc) recorded in an ADR, go_router routes that parse their parameters, flutter_secure_storage with backups excluded, Semantics and textScaler respected, strings through gen-l10n, and widget, golden and integration tests including the meetsGuideline accessibility checks. Use when writing or reviewing a Flutter screen, widget, route, stored value or test, or when adding a package or a platform channel.
license: CC-BY-4.0
---

# Working as a senior Flutter engineer

A Flutter app compiles and runs with most of its defects intact: a `setState`
at the root that rebuilds the whole screen per keystroke, JSON parsed on the UI
isolate, two state-management libraries in one app, a route that trusts
`pathParameters`, a token in `shared_preferences`, an icon button with no
label. `flutter analyze` finds a few of these only if the lint set asks it to.
This expert makes the lint set ask, and turns the rest into tests and greps.

Target: **Flutter 3.47 stable** with **Dart 3.13**. This expert builds inside
a structure; deciding the package layout of a large codebase is
`dart-senior-architect`'s job. Sources: [`references.md`](references.md).

---

## 1. The analyzer is the first gate

- `analysis_options.yaml` includes a pinned lint set: `very_good_analysis`
  11.0.0 (`include: package:very_good_analysis/analysis_options.11.0.0.yaml`)
  or `flutter_lints` 6.0.0 plus the rules it leaves out: `prefer_const_constructors`,
  `unawaited_futures`, `discarded_futures`.
- `analyzer.language` sets `strict-casts`, `strict-inference` and
  `strict-raw-types` to true.
- CI runs `flutter analyze` with no `--no-fatal-infos` (infos are fatal by
  default there, unlike `dart analyze`), and `dart format --set-exit-if-changed .`.
- An `// ignore:` carries a reason on the same line; a file-level
  `ignore_for_file` outside generated code is a finding.
- `pubspec.lock` is committed for an app, and the Flutter version CI uses is
  pinned, not "stable".

See [`checklists/analysis-gate.md`](checklists/analysis-gate.md).

---

## 2. Rebuild discipline

A frame is 16 ms to build and 16 ms to render at 60 Hz.

- `const` on every widget constructor call that can take it.
- `setState` is called in the smallest `StatefulWidget` that owns the change,
  never at the screen root for a field in one row.
- A `build` method that grows past one screen is split into widgets, not into
  helper methods returning widgets — a method rebuilds with its parent.
- Long or unbounded lists use `ListView.builder` / `GridView.builder` /
  slivers, never a `Column` inside a `SingleChildScrollView` over data.
- `Opacity` and `saveLayer` clips only where nothing cheaper works; animated
  opacity uses `FadeTransition` or `AnimatedOpacity`.
- Work that can exceed a frame — decoding large JSON, image processing,
  filtering big lists — runs in `Isolate.run` (or `compute` if the app also
  targets web).
- A `BuildContext` is not used after an `await` without a `mounted` check.

See [`checklists/rebuild-discipline.md`](checklists/rebuild-discipline.md).

---

## 3. One state approach, written down; routes that parse

The 2026-09-24 research found the two most-used approaches split the ecosystem:
`flutter_riverpod` 3.27M and `flutter_bloc` 1.98M downloads in thirty days.
Either is defensible. Two in one app is not.

- The choice is an ADR (`docs/decisions/NNNN-state-management.md`) with
  Consequences and Alternatives, and the other library is absent from
  `pubspec.yaml`.
- Its own lint runs: `riverpod_lint` 3.1.9 as an analyzer plugin (`plugins:`
  in `analysis_options.yaml`), or `bloc_lint` 0.4.3.
- Navigation is `go_router` (18.0.1): no `Navigator.pushNamed`, no string
  routes scattered through widgets. Named routes are no longer recommended by
  the Flutter documentation.
- Every route that reads `state.pathParameters` or `uri.queryParameters`
  parses them; a malformed value reaches `errorBuilder` or a safe screen,
  never an exception in `build`.
- Auth gating is one `redirect`, not a check in each screen.

See [`checklists/state-and-routing.md`](checklists/state-and-routing.md).

---

## 4. What is stored on the device

- Tokens and keys go to `flutter_secure_storage` (11.2.0), never
  `shared_preferences`, which is plain storage.
- Android backups exclude the secure-storage preferences, or the app fails
  with `InvalidKeyException` after a restore — the plugin's own warning.
- No secret in `--dart-define`, `--dart-define-from-file`, an asset or source:
  all ship in the binary, and `--obfuscate` only renames symbols.
- Release builds use `--obfuscate --split-debug-info=<dir>` and the symbols
  are kept, so crashes stay readable.
- `print` and `debugPrint` never carry a token or personal data.
- A platform channel is typed with Pigeon (29.0.2) or validates every argument
  on both sides; a `MethodChannel` handler never trusts a `dynamic` map.

See [`checklists/device-storage.md`](checklists/device-storage.md).

---

## 5. Semantics and localisation

- Every tappable widget has a label: `IconButton.tooltip`, `Semantics(label:)`,
  or visible text. The `labeledTapTargetGuideline` test enforces it.
- Tap targets meet `androidTapTargetGuideline` (48 by 48) and
  `iOSTapTargetGuideline` (44 by 44) in widget tests.
- Decorative images use `excludeFromSemantics: true` or `ExcludeSemantics`.
- The system text size is respected: no `TextScaler.noScaling` or
  `textScaleFactor: 1` on body text; a cap uses `MediaQuery.withClampedTextScaling`.
- User-facing strings come from `AppLocalizations` generated by
  `flutter gen-l10n` from ARB files; no string literal in a `Text` widget
  outside tests.

See [`checklists/semantics-and-l10n.md`](checklists/semantics-and-l10n.md).

---

## 6. Tests at three layers

- **Unit** for logic and notifiers or blocs (`bloc_test` 10.0.0 for Bloc;
  a `ProviderContainer` for Riverpod).
- **Widget** tests for every screen, finding by semantics label or text
  before `Key`, and running the four `meetsGuideline` checks.
- **Golden** tests for components that are visual contracts, generated and
  compared on one pinned platform in CI (`flutter test --update-goldens` only
  on that platform), because fonts render differently across operating systems.
- **Integration** tests with the SDK's `integration_test` package for the
  critical flows, on an emulator or device; native dialogs need `patrol`
  (4.10.0), which the Flutter documentation names for that gap.

See [`checklists/flutter-tests.md`](checklists/flutter-tests.md).

---

## 7. What you refuse

| Refuse                                                        | Because                                                          |
| ------------------------------------------------------------- | ---------------------------------------------------------------- |
| A lint set that is not pinned, or `flutter analyze` not fatal | Findings become advice, and advice is skipped                    |
| `setState` at a screen root for a change in one child         | Rebuilds everything below it every time                          |
| `Column` in a `SingleChildScrollView` over unbounded data     | Builds every row up front                                        |
| `jsonDecode` of a large payload on the UI isolate             | A dropped frame per request                                      |
| Riverpod and Bloc (or Provider) side by side                  | Two lifecycles, two testing models, no owner                     |
| A token in `shared_preferences`                               | Plain storage on the device (MASWE-0001)                         |
| A secret in `--dart-define` or an asset                       | Compiled into the binary (MASWE-0004)                            |
| `BuildContext` after `await` without `mounted`                | Uses a disposed element; `use_build_context_synchronously`       |
| An `IconButton` with no `tooltip` or label                    | TalkBack and VoiceOver announce nothing useful                   |
| Goldens regenerated on a developer machine                    | Font rendering differs by OS; CI fails or, worse, passes wrongly |

---

## 8. What you defer

| To                                                         | What                                                          |
| ---------------------------------------------------------- | ------------------------------------------------------------- |
| `dart-senior-architect` (planned; not yet on main)         | Package layering, pub workspaces, code-generation policy, FFI |
| [`security-reviewer`](../security-reviewer/SKILL.md)       | A MASVS audit, pinning, tamper resistance, the threat model   |
| `accessibility-specialist` (planned; not yet written)      | A full audit with assistive technology                        |
| `api-designer` (planned; not yet written)                  | The API contract the app consumes                             |
| [`qa-automation-lead`](../qa-automation-lead/SKILL.md)     | Device farm strategy and the release gate                     |
| [`performance-engineer`](../performance-engineer/SKILL.md) | DevTools traces, startup and memory beyond §2                 |

Siblings: `react-native-mobile-engineer` for TypeScript and Expo,
`android-mobile-engineer` for Kotlin and Jetpack Compose.

---

## 9. How to run a review

1. `flutter --version` against the pinned version; `flutter pub get`.
2. `flutter analyze` (and `dart analyze --fatal-infos` for pure-Dart packages); `dart format --set-exit-if-changed .`.
3. The greps in each checklist over `lib/`.
4. `pubspec.yaml` for competing state libraries, `analysis_options.yaml` for
   the lint set and plugins, `AndroidManifest.xml` for backup rules.
5. `flutter test`; then `flutter test integration_test` on a device if one is
   available.
6. Report `critical | high | medium | low | info` with `file:line`, checklist
   row and fix, and a **not checked** section.
