# Package layering

In Dart the package is the only boundary the toolchain enforces: an import of a
package not in `pubspec.yaml` does not resolve, and `lib/src/` is private by
convention that a lint can make a rule. Layering that should hold is therefore
expressed in packages first and lint rules second.

| #   | Check                                                                     | How                                                                                                | Source                                          |
| --- | ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| PL1 | The package graph and its allowed dependencies are recorded in an ADR     | the ADR; compare with the `dependencies:` of each member                                           | Nygard ADR format; Dart 3.13 — package layout   |
| PL2 | The domain package depends on no Flutter SDK and no data or UI package    | read its `pubspec.yaml`: no `flutter: sdk: flutter`, no sibling that sits above it                 | Dart 3.13 — package layout; pub workspaces      |
| PL3 | No import of another package's `lib/src/`; `implementation_imports` is on | `grep -rnE "import 'package:[a-z_]+/src/" lib` shows only the package's own name                   | Dart 3.13 — package layout; lints 6.1.0         |
| PL4 | Every imported package is declared; `depend_on_referenced_packages` is on | read `analysis_options.yaml`; `dart analyze --fatal-infos`                                         | lints 6.1.0 (core); Dart 3.13 — linter rules    |
| PL5 | No missing, unused, under- or over-promoted dependency                    | `dart run dependency_validator` in each package (a dev dependency)                                 | dependency_validator 5.1.0                      |
| PL6 | Directory rules inside a package are an analyzer plugin at error severity | `plugins:` lists `import_lint` (or a project plugin) and its rules name `target`, `from`, `except` | import_lint 2.0.0; Dart 3.13 — analyzer plugins |
| PL7 | `always_use_package_imports` (or its opposite) is chosen once and on      | read `analysis_options.yaml`                                                                       | Dart 3.13 — linter rules                        |
| PL8 | Each package's public surface is what `lib/<package>.dart` exports        | read the top-level library for `export` of `src/` files; no consumer reaches past it               | Dart 3.13 — package layout                      |

## Why each one

**PL2** is the rule that makes the rest cheap. When the domain package's
`pubspec.yaml` does not list Flutter or the data package, an import of either
is a resolution error, not a review comment — and the domain stays testable
with `dart test` on any machine.

**PL3** because `lib/src/` is how a Dart package says "not API". A consumer that
imports it is coupled to whatever the author refactors next; the lint turns
that into an analyzer error.

**PL6** covers what packages cannot: rules between directories of one package.
Analyzer plugins run inside `dart analyze` and `flutter analyze` since Dart
3.10, so the rule fails the same command CI already runs.
