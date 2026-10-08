# Analysis gate

Dart's analyzer can refuse most of what goes wrong in a Flutter codebase before
it runs. It only does so for the rules somebody turned on, and only fails a
build for the severities somebody made fatal.

| #   | Check                                                                                                     | How                                                                                               | Source                                                             |
| --- | --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| AG1 | `analysis_options.yaml` includes a pinned lint set                                                        | `grep -n "include:" analysis_options.yaml` names `very_good_analysis` 11.0.0 or `flutter_lints`   | very_good_analysis 11.0.0; flutter_lints 6.0.0                     |
| AG2 | With `flutter_lints`, `prefer_const_constructors`, `unawaited_futures` and `discarded_futures` are added  | read `linter.rules`; `flutter_lints` 6.0.0 enables only `prefer_const_constructors_in_immutables` | flutter_lints 6.0.0; very_good_analysis 11.0.0                     |
| AG3 | `strict-casts`, `strict-inference` and `strict-raw-types` are true                                        | read `analyzer.language`                                                                          | Dart 3.13 — customizing static analysis                            |
| AG4 | CI runs `flutter analyze` without `--no-fatal-infos`; pure-Dart packages run `dart analyze --fatal-infos` | read the workflow; run it                                                                         | Flutter 3.47 — `flutter analyze`                                   |
| AG5 | CI runs `dart format --set-exit-if-changed .`                                                             | read the workflow; run it                                                                         | Dart 3.13 — `dart format`                                          |
| AG6 | Every `// ignore:` carries a reason; no `ignore_for_file` outside generated files                         | `grep -rn "ignore_for_file\|// ignore:" lib`                                                      | Dart 3.13 — customizing static analysis                            |
| AG7 | The chosen state library's lint runs: `riverpod_lint` under `plugins:`, or `bloc_lint`                    | read `analysis_options.yaml`                                                                      | riverpod_lint 3.1.9; bloc_lint 0.4.3; Dart 3.13 — analyzer plugins |
| AG8 | `pubspec.lock` is committed, and the Flutter version CI uses is pinned                                    | `git ls-files pubspec.lock`; read the workflow's Flutter setup step                               | Dart 3.13 — package dependencies                                   |

## Why each one

**AG2** is the surprise. `prefer_const_constructors` is the rule the Flutter
performance page points at, and `flutter_lints` 6.0.0 does not turn it on: it
ships `prefer_const_constructors_in_immutables` only. `very_good_analysis`
11.0.0 enables all three rules in the row.

**AG3** because without `strict-casts` a `dynamic` from `jsonDecode` flows into
a typed field with no cast and no warning, and the failure is a runtime
`TypeError` on the first unexpected payload.

**AG4** because the two analyzers disagree on the default. `flutter analyze`
treats infos as fatal unless told `--no-fatal-infos`; `dart analyze` exits 0 on
infos unless told `--fatal-infos`. Most lints report at info level, so a
pure-Dart package in the same repository passes CI with every lint firing.
