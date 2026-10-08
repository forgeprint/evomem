# References

Every rule in [`SKILL.md`](SKILL.md) and every checklist row rests on one of
these. A row that cites nothing is somebody's opinion and does not belong in a
review ([ADR 0010](../../docs/decisions/0010-review-standards.md)).

Package versions were read from the pub.dev API and the Dart and Flutter
versions from their release manifests on the date shown; dart.dev pages carry a
"last updated" date that is recorded where present. **Re-check every 90 days**,
and when the next Dart stable ships.

> **Next re-check due: 2026-12-24.**

## Language and tools

| Short name                              | Reference                                                                                                      | Version                                                                                 | Checked    | Used for                                                                                                  |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------- |
| Dart 3.13                               | [Dart SDK archive](https://dart.dev/get-dart/archive)                                                          | 3.13.4 stable, 2026-09-15                                                               | 2026-09-25 | The target in SKILL.md                                                                                    |
| Dart 3.13 — package layout              | [Package layout conventions](https://dart.dev/tools/pub/package-layout)                                        | page for Dart 3.13.3, last updated 2026-08-31                                           | 2026-09-25 | SKILL.md §2, §7; PL1–PL3, PL8, FB2, PP6: `lib/src` is not public API                                      |
| Dart 3.13 — linter rules                | [Linter rules](https://dart.dev/tools/linter-rules)                                                            | Dart 3.13                                                                               | 2026-09-25 | PL4, PL7: `depend_on_referenced_packages`, `always_use_package_imports`                                   |
| lints 6.1.0                             | [lints](https://pub.dev/packages/lints)                                                                        | 6.1.0: `depend_on_referenced_packages` in core, `implementation_imports` in recommended | 2026-09-25 | PL3, PL4, PP8                                                                                             |
| Dart 3.13 — customizing static analysis | [Customizing static analysis](https://dart.dev/tools/analysis)                                                 | Dart 3.13                                                                               | 2026-09-25 | TC3, TC4, CG8: strict modes, `analyzer.exclude`                                                           |
| Dart 3.13 — analyzer plugins            | [Analyzer plugins](https://dart.dev/tools/analyzer-plugins)                                                    | supported since Dart 3.10                                                               | 2026-09-25 | SKILL.md §2; PL6: `plugins:`, reported by `dart analyze` and `flutter analyze`                            |
| Dart 3.13 — class modifiers             | [Class modifiers](https://dart.dev/language/class-modifiers)                                                   | language 3.0+; last updated 2026-05-12                                                  | 2026-09-25 | SKILL.md §5; TC1, TC2, TC7                                                                                |
| Dart 3.13 — patterns                    | [Patterns](https://dart.dev/language/patterns)                                                                 | language 3.0+                                                                           | 2026-09-25 | TC2: exhaustive `switch`                                                                                  |
| Dart 3.13 — null safety                 | [Sound null safety](https://dart.dev/null-safety)                                                              | Dart 3.13                                                                               | 2026-09-25 | TC5, TC6                                                                                                  |
| import_lint 2.0.0                       | [import_lint](https://pub.dev/packages/import_lint)                                                            | 2.0.0; an analyzer plugin, needs Dart 3.10+                                             | 2026-09-25 | PL6. Small (about 6k downloads at check); a project plugin on `analysis_server_plugin` is the alternative |
| analysis_server_plugin 0.3.23           | [analysis_server_plugin](https://pub.dev/packages/analysis_server_plugin)                                      | 0.3.23                                                                                  | 2026-09-25 | PL6: writing a project's own rules                                                                        |
| dependency_validator 5.1.0              | [dependency_validator](https://pub.dev/packages/dependency_validator)                                          | 5.1.0; supports pub workspaces                                                          | 2026-09-25 | SKILL.md §2; PL5                                                                                          |
| Nygard ADR format                       | [Documenting architecture decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) | original, 2011                                                                          | 2026-09-25 | SKILL.md §1; PL1, CG1                                                                                     |

## Workspace and versions

| Short name                      | Reference                                                          | Version                                                  | Checked    | Used for                                                          |
| ------------------------------- | ------------------------------------------------------------------ | -------------------------------------------------------- | ---------- | ----------------------------------------------------------------- |
| Dart 3.13 — pub workspaces      | [Pub workspaces](https://dart.dev/tools/pub/workspaces)            | since Dart 3.6.0; last updated 2026-05-15                | 2026-09-25 | SKILL.md §3; PW1–PW4, PL2                                         |
| Melos 8.9.0                     | [melos](https://pub.dev/packages/melos)                            | 8.9.0; built on pub workspaces, config in `pubspec.yaml` | 2026-09-25 | PW4, PW5                                                          |
| Dart 3.13 — what not to commit  | [What not to commit](https://dart.dev/tools/pub/private-files)     | current at check                                         | 2026-09-25 | PW6, CG4: applications commit `pubspec.lock`                      |
| Dart 3.13 — versioning          | [Package versioning](https://dart.dev/tools/pub/versioning)        | last updated 2026-05-15                                  | 2026-09-25 | SKILL.md §7; PW7, PP4, PP8, TC7: caret ranges, the 0.x convention |
| `dart pub downgrade`            | [dart pub downgrade](https://dart.dev/tools/pub/cmd/pub-downgrade) | Dart 3.13                                                | 2026-09-25 | PW7: resolves the lowest allowed versions                         |
| Dart 3.13 — `dart pub outdated` | [dart pub outdated](https://dart.dev/tools/pub/cmd/pub-outdated)   | Dart 3.13                                                | 2026-09-25 | PW8                                                               |

## Code generation

| Short name               | Reference                                                                                                      | Version                 | Checked    | Used for                                                                                                                      |
| ------------------------ | -------------------------------------------------------------------------------------------------------------- | ----------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------- |
| build_runner 2.16.1      | [build_runner](https://pub.dev/packages/build_runner) and [the Dart page](https://dart.dev/tools/build_runner) | 2.16.1                  | 2026-09-25 | SKILL.md §4; CG1, CG3, CG4, CG6                                                                                               |
| build_verify 3.1.1       | [build_verify](https://pub.dev/packages/build_verify)                                                          | 3.1.1                   | 2026-09-25 | CG2, CG9: `expectBuildClean`                                                                                                  |
| freezed 4.0.2            | [freezed](https://pub.dev/packages/freezed)                                                                    | 4.0.2; needs Dart 3.13  | 2026-09-25 | CG7                                                                                                                           |
| json_serializable 6.14.1 | [json_serializable](https://pub.dev/packages/json_serializable)                                                | 6.14.1                  | 2026-09-25 | CG7                                                                                                                           |
| Dart 3.13 — publishing   | [Publishing packages](https://dart.dev/tools/pub/publishing)                                                   | last updated 2026-05-15 | 2026-09-25 | SKILL.md §4, §7; CG5, PP1–PP3, PP7: `.gitignore` and `.pubignore` exclude files from the upload; retraction within seven days |

## Native interop

| Short name                              | Reference                                                                                                  | Version                                         | Checked    | Used for                                       |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------------- | ---------- | ---------------------------------------------- |
| Dart 3.13 — C interop                   | [C interop using dart:ffi](https://dart.dev/interop/c-interop)                                             | current at check                                | 2026-09-25 | SKILL.md §6; FB1, FB2: `ffigen` for large APIs |
| ffigen 22.0.0                           | [ffigen](https://pub.dev/packages/ffigen)                                                                  | 22.0.0                                          | 2026-09-25 | FB3                                            |
| ffi 2.2.0                               | [ffi](https://pub.dev/packages/ffi)                                                                        | 2.2.0                                           | 2026-09-25 | FB5: `calloc`, `malloc`, `using`, `Arena`      |
| Dart 3.13 — hooks                       | [Hooks](https://dart.dev/tools/hooks)                                                                      | build hooks since Dart 3.10; link hooks in 3.13 | 2026-09-25 | FB4, FB7                                       |
| hooks 2.2.0; native_toolchain_c 0.19.5  | [hooks](https://pub.dev/packages/hooks), [native_toolchain_c](https://pub.dev/packages/native_toolchain_c) | 2.2.0; 0.19.5                                   | 2026-09-25 | FB4                                            |
| Flutter 3.47 — concurrency and isolates | [Concurrency and isolates](https://docs.flutter.dev/perf/isolates)                                         | Flutter 3.47                                    | 2026-09-25 | FB6                                            |
| pana 0.23.19                            | [pana](https://pub.dev/packages/pana)                                                                      | 0.23.19                                         | 2026-09-25 | PP5: the pub points score before release       |

## Deferred to elsewhere

- Building screens, state management and tests inside the app:
  `flutter-mobile-engineer`, proposed in a separate pull request.
- Application security: [`security-reviewer`](../security-reviewer/SKILL.md).
- CI pipelines and release automation:
  [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md).
