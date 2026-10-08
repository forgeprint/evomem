# Pub workspace

A repository of several Dart packages used to need path overrides or a tool to
link them. Since Dart 3.6 pub resolves them together as a workspace, with one
lockfile, and the older mechanisms now hide the real dependency graph.

| #   | Check                                                                                               | How                                                                                                 | Source                                       |
| --- | --------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| PW1 | A multi-package repository is a pub workspace: `workspace:` in the root `pubspec.yaml`              | read the root `pubspec.yaml`                                                                        | Dart 3.13 — pub workspaces                   |
| PW2 | Every member has `resolution: workspace` and an SDK constraint of `^3.6.0` or higher                | `grep -L "resolution: workspace" packages/*/pubspec.yaml` returns nothing                           | Dart 3.13 — pub workspaces                   |
| PW3 | There is one `pubspec.lock`, at the root                                                            | `find . -name pubspec.lock -not -path "./pubspec.lock" -not -path "*/.dart_tool/*"` returns nothing | Dart 3.13 — pub workspaces                   |
| PW4 | No `pubspec_overrides.yaml` and no `path:` dependency between members                               | `find . -name pubspec_overrides.yaml`; `grep -rn "path:" packages/*/pubspec.yaml`                   | Dart 3.13 — pub workspaces; Melos 8.9.0      |
| PW5 | If Melos is used, it is configured under `melos:` in the root `pubspec.yaml`, with no `melos.yaml`  | `ls melos.yaml` fails; read the root `pubspec.yaml`                                                 | Melos 8.9.0                                  |
| PW6 | The lockfile is committed for an application workspace                                              | `git ls-files pubspec.lock`                                                                         | Dart 3.13 — what not to commit               |
| PW7 | Dependency constraints use caret ranges, and a published package's lower bounds are exercised in CI | read constraints; CI runs `dart pub downgrade` then analysis and tests for published packages       | Dart 3.13 — versioning; `dart pub downgrade` |
| PW8 | `dart pub outdated` is reviewed on a schedule, and upgrades land one major at a time                | the dependency audit names the date it was run                                                      | Dart 3.13 — `dart pub outdated`              |

## Why each one

**PW3** is the property the workspace exists for: one resolution for every
package, so two members cannot silently build against different versions of the
same dependency.

**PW4** because overrides and path dependencies were the pre-workspace way to
link packages, and left in place they make the graph in `pubspec.yaml` differ
from the graph the build uses. Melos itself moved to pub workspaces and stopped
writing override files.

**PW7** because a caret lower bound nobody has tested is a claim. `dart pub
downgrade` resolves the lowest allowed versions, and running the suite on them
is the only way to know the claim holds for a consumer who has them.
