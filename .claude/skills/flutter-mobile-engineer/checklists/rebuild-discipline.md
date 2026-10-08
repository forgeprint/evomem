# Rebuild discipline

Flutter redraws by rebuilding widgets. What decides whether a screen holds 60
frames a second is how much gets rebuilt per change and what runs on the UI
isolate while it happens.

| #    | Check                                                                                               | How                                                                                            | Source                                         |
| ---- | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| RD1  | `const` is used wherever a constructor allows it                                                    | `prefer_const_constructors` is on (analysis-gate AG2) and `flutter analyze` is clean           | Flutter 3.47 — performance best practices      |
| RD2  | `setState` is called in the smallest widget that owns the change                                    | `grep -rn "setState(" lib`; for each, read what the state is and how much of the tree is below | Flutter 3.47 — performance best practices      |
| RD3  | Large `build` methods are split into widget classes, not helper methods returning `Widget`          | `grep -rnE "Widget _build\w*\(" lib`; each hit that is large or stateful becomes a widget      | Flutter 3.47 — performance best practices      |
| RD4  | Long or unbounded lists use a lazy builder or slivers                                               | `grep -rn -A6 "SingleChildScrollView" lib \| grep -E "Column\|\.map\("`                        | Flutter 3.47 — performance best practices      |
| RD5  | No `Opacity` wrapping an animation or a large subtree; `FadeTransition` / `AnimatedOpacity` instead | `grep -rn "Opacity(" lib`                                                                      | Flutter 3.47 — performance best practices      |
| RD6  | No `Clip.antiAliasWithSaveLayer` without a stated reason                                            | `grep -rn "antiAliasWithSaveLayer" lib`                                                        | Flutter 3.47 — performance best practices      |
| RD7  | Work that can exceed a frame runs in `Isolate.run` or `compute`                                     | `grep -rn "jsonDecode\|decodeImage" lib`; read the payload size at each call                   | Flutter 3.47 — concurrency and isolates        |
| RD8  | A background isolate that calls a plugin initialises `BackgroundIsolateBinaryMessenger`             | read each `Isolate.spawn` entry point                                                          | Flutter 3.47 — concurrency and isolates        |
| RD9  | No `BuildContext` use after `await` without a `mounted` check                                       | `use_build_context_synchronously` is on and clean                                              | flutter_lints 6.0.0; very_good_analysis 11.0.0 |
| RD10 | Controllers, streams and subscriptions created in `initState` are disposed in `dispose`             | `grep -rnE "(Controller\|StreamSubscription)\(" lib`; match each to a `dispose()` call         | Flutter 3.47 — `State.dispose` API             |

## Why each one

**RD2** is the most common Flutter performance defect an agent writes, because
it is the pattern in the counter example. A form whose root `State` calls
`setState` on each keystroke rebuilds every field, image and list on the
screen for one character.

**RD7** because a 2 MB JSON response decoded on the UI isolate is a visible
freeze, and it never shows up in a test with a ten-item fixture.

**RD3** because a helper method is not a widget: it has no element of its own,
cannot be `const`, and rebuilds whenever its parent does.
