# Flutter tests

Flutter's test runner has three layers built in, and each catches a different
kind of defect. The rows below are about using each for what it is good at, and
keeping the one that is sensitive to the platform — goldens — deterministic.

| #    | Check                                                                                                          | How                                                                                     | Source                                          |
| ---- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ----------------------------------------------- |
| FT1  | `flutter test` passes and runs in CI                                                                           | run it; read the workflow                                                               | Flutter 3.47 — testing overview                 |
| FT2  | Notifiers or blocs have unit tests; Bloc uses `blocTest`, Riverpod a `ProviderContainer`                       | `grep -rn "blocTest\|ProviderContainer" test`                                           | bloc_test 10.0.0; flutter_riverpod 3.4.3        |
| FT3  | Every screen has a widget test                                                                                 | compare the screens under `lib/` with `test/`                                           | Flutter 3.47 — testing overview                 |
| FT4  | Widget tests find by text or semantics label before `Key`                                                      | `grep -rc "find.byKey" test` against `find.text\|find.bySemanticsLabel`                 | Flutter 3.47 — testing overview                 |
| FT5  | Golden files are generated and compared on one pinned platform in CI, never regenerated locally                | read the workflow; `git log -- '**/goldens/**'` shows updates from CI or its image only | Flutter 3.47 — `matchesGoldenFile` API          |
| FT6  | Goldens that load real fonts say so; the rest use the default test font                                        | read the golden test setup                                                              | Flutter 3.47 — `matchesGoldenFile` API          |
| FT7  | Critical flows have `integration_test` tests, run with `flutter test integration_test` on a device or emulator | `ls integration_test/`; read the workflow                                               | Flutter 3.47 — testing overview                 |
| FT8  | Flows that meet a native permission dialog or notification use `patrol`, or are listed as not covered          | read the integration tests for permission prompts                                       | Flutter 3.47 — testing overview; patrol 4.10.0  |
| FT9  | Routes with parameters have a test with a malformed value                                                      | read the tests for the routes in `state-and-routing.md` SR6                             | MASVS-CODE-4                                    |
| FT10 | Tests use `mocktail` or fakes for services, never a real network                                               | `grep -rn "http\.\|Dio(" test`                                                          | mocktail 1.0.5; Flutter 3.47 — testing overview |

## Why each one

**FT5** is the row that decides whether goldens are useful or noise. Flutter's
own documentation warns that fonts render differently across platforms and
Flutter versions; a golden generated on a developer's laptop fails on CI, and
the easy fix — regenerate on CI and commit — makes the test compare CI with
itself.

**FT8** because `integration_test` cannot tap a system permission dialog. A flow
that needs one is either driven by a tool that can, or it is listed as not
covered; it is not silently skipped.

**FT4** because a test that finds a button by `Key` passes when the button has
no label, and one that finds it by label fails — which is what connects the
semantics checklist to CI.
