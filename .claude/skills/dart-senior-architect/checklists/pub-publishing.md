# Pub publishing

A version on pub.dev lasts forever; it can be retracted for seven days, not
removed. Everything a published package promises is therefore decided before
`dart pub publish`, and checked by commands that do not publish.

| #   | Check                                                                                                   | How                                                                                            | Source                              |
| --- | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ----------------------------------- |
| PP1 | Packages that must not be published say `publish_to: none`                                              | `grep -L "publish_to: none" packages/*/pubspec.yaml` lists only the packages meant for pub.dev | Dart 3.13 — publishing              |
| PP2 | `dart pub publish --dry-run` is clean in CI for every published package                                 | read the workflow; run it                                                                      | Dart 3.13 — publishing              |
| PP3 | Each published package has `LICENSE`, `README.md` and a `CHANGELOG.md` entry for the version            | `ls`; the CHANGELOG's top entry matches `version:` in `pubspec.yaml`                           | Dart 3.13 — publishing              |
| PP4 | Versions follow semantic versioning, with the Dart convention for 0.x: a minor bump is breaking         | compare the ABI-level change in the release with the version bump                              | Dart 3.13 — versioning              |
| PP5 | `pana` reports no lost points the team has not accepted in writing                                      | `dart pub global run pana` (0.23.19) on the package before release                             | pana 0.23.19                        |
| PP6 | The public API is exported from `lib/<package>.dart`; nothing a consumer needs lives only in `lib/src/` | read the exports; the example in `example/` imports only the top-level library                 | Dart 3.13 — package layout          |
| PP7 | A bad release is retracted within seven days and followed by a fixed version                            | the release runbook names the retraction step                                                  | Dart 3.13 — publishing              |
| PP8 | Removing or renaming public API is preceded by a `@Deprecated` release                                  | `grep -rn "@Deprecated" lib`; the CHANGELOG names the version that removes it                  | Dart 3.13 — versioning; lints 6.1.0 |

## Why each one

**PP4** because every consumer depends on a caret range. A breaking change
released as a minor — or, before 1.0, as a patch — is pulled into every
consumer's next `pub get` without anyone choosing it.

**PP2** is cheap and catches the failures that cannot be undone: missing
files, files that should not be there, an invalid pubspec, and — with
`codegen-policy.md` CG5 — generated files left out of the upload.

**PP7** because "a published package lasts forever". Retraction is the only
tool, it has a seven-day window, and it is only usable if somebody knew it
existed before the bad release.
