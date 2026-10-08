# State and routing

Flutter leaves both decisions open: how state is held and how screens are
reached. Each is cheap to decide once and expensive to have decided twice.

| #    | Check                                                                                               | How                                                                                                                               | Source                                         |
| ---- | --------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| SR1  | The state-management approach is recorded in an ADR with Consequences and Alternatives              | `ls docs/decisions/` for it                                                                                                       | Flutter 3.47 — state management options        |
| SR2  | Only one approach is a dependency                                                                   | `grep -nE "^\s+(flutter_riverpod\|riverpod\|hooks_riverpod\|flutter_bloc\|bloc\|provider\|get):" pubspec.yaml` returns one family | Flutter 3.47 — state management options        |
| SR3  | Riverpod: providers are not created inside `build`; `ref.watch` in `build`, `ref.read` in callbacks | `riverpod_lint` clean under `dart analyze`                                                                                        | riverpod_lint 3.1.9; flutter_riverpod 3.4.3    |
| SR4  | Bloc: widgets dispatch events and read state; no business logic in `onPressed`                      | `bloc_lint` clean; read `onPressed` bodies for more than one call                                                                 | bloc_lint 0.4.3; flutter_bloc 9.1.1            |
| SR5  | Navigation goes through `go_router`; no `Navigator.pushNamed` or `routes:` map on `MaterialApp`     | `grep -rnE "pushNamed\|routes:\s*\{" lib`                                                                                         | go_router 18.0.1; Flutter 3.47 — deep linking  |
| SR6  | Every route that reads `pathParameters` or `queryParameters` parses them and handles a bad value    | `grep -rn "pathParameters\|queryParameters" lib`; each value goes through a parser, not `!` or `int.parse` alone                  | go_router 18.0.1; MASVS-CODE-4                 |
| SR7  | Unknown locations reach `errorBuilder` (or `onException`), not the framework's default error page   | read the `GoRouter` constructor                                                                                                   | go_router 18.0.1                               |
| SR8  | Authentication gating is one top-level `redirect`, not repeated checks inside screens               | `grep -rn "redirect:" lib`; read screens for their own auth checks                                                                | go_router 18.0.1                               |
| SR9  | A deep link never triggers a state change on arrival without the user confirming                    | read screens reachable by a link for writes in `initState`                                                                        | MASWE-0029; MASVS-PLATFORM-1                   |
| SR10 | Links to the app's own domain are verified App Links / Universal Links, not a custom scheme only    | read `AndroidManifest.xml` intent filters (`autoVerify`) and iOS associated domains                                               | Flutter 3.47 — deep linking; Android App Links |

## Why each one

**SR2** is the one this expert refuses outright. Riverpod and Bloc each bring
their own lifecycle, their own testing model and their own answer to "where does
this state live". An app with both has two answers, and every new screen picks
whichever its author knew.

**SR6** because `int.parse(state.pathParameters['id']!)` is the pattern an agent
writes, and a link with `id=abc` turns it into an exception during a build — a
red screen in debug, a blank one in release.

**SR8** because a check repeated in each screen is a check missing from the
next screen somebody adds.
