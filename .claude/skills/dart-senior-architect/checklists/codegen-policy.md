# Code-generation policy

Dart has no reflection in Flutter builds, so models, serialisers and routes are
often generated. The architectural question is not whether to generate but
where the outputs live and how anyone knows they are current.

| #   | Check                                                                                                                   | How                                                                                      | Source                                              |
| --- | ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | --------------------------------------------------- |
| CG1 | An ADR names the generators and whether their outputs are committed                                                     | the ADR; compare with `dev_dependencies` that are builders                               | Nygard ADR format; build_runner 2.16.1              |
| CG2 | If outputs are committed, a `build_verify` test fails when they are stale                                               | `grep -rn "expectBuildClean" test`                                                       | build_verify 3.1.1                                  |
| CG3 | If outputs are not committed, CI runs `dart run build_runner build --delete-conflicting-outputs` before analysis        | read the workflow order                                                                  | build_runner 2.16.1                                 |
| CG4 | The ignore rules match the policy: generated patterns are in `.gitignore` only when outputs are not committed           | `grep -nE "\.g\.dart\|\.freezed\.dart" .gitignore`                                       | build_runner 2.16.1; Dart 3.13 — what not to commit |
| CG5 | A published package includes its generated files in the upload                                                          | `dart pub publish --dry-run` lists the `.g.dart` / `.freezed.dart` files                 | Dart 3.13 — publishing                              |
| CG6 | Each builder is limited with `generate_for` in `build.yaml`                                                             | read `build.yaml`                                                                        | build_runner 2.16.1                                 |
| CG7 | Generator versions are pinned together: `freezed` with `freezed_annotation`, `json_serializable` with `json_annotation` | read `pubspec.yaml` and the lockfile                                                     | freezed 4.0.2; json_serializable 6.14.1             |
| CG8 | Generated files are excluded from analysis and coverage, not from the build                                             | read `analyzer.exclude` in `analysis_options.yaml` and the coverage filter               | Dart 3.13 — customizing static analysis             |
| CG9 | Nothing hand-edits a generated file                                                                                     | `git log --format=%an -- '*.g.dart'` shows only generator runs, or CG2's test would fail | build_verify 3.1.1                                  |

## Why each one

**CG5** is the failure that reaches other people. Pub publishes everything
under the package root except hidden files and what `.gitignore` or
`.pubignore` excludes. A package whose generated files are git-ignored uploads
without them, and every consumer gets a package that does not compile.

**CG2** is what makes "committed" safe. Without it, the committed outputs are
whatever the last person to run the generator produced, and a model change
without a regeneration passes review.

**CG1** because the two policies are both defensible and cannot be mixed: half
the team committing and half ignoring produces merge conflicts in files nobody
wrote.
