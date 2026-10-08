# Type contracts

Dart 3 gives a package author a way to say, in the type, what a caller may do
with a class — extend it, implement it, neither — and a sound type system to
make the rest of the contract hold. A public API that does not use them has
made promises it did not mean to.

| #   | Check                                                                                                                                         | How                                                                 | Source                                  |
| --- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | --------------------------------------- |
| TC1 | Every public class in a shared or published package carries a modifier: `final`, `interface`, `base`, `sealed` or `abstract` with one of them | `grep -rnE "^class " lib` in the package; each hit is a finding     | Dart 3.13 — class modifiers             |
| TC2 | Closed sets of states are `sealed`, and `switch` over them has no default                                                                     | `grep -rn -B6 "default:\|_ =>" lib`; check the subject's type       | Dart 3.13 — class modifiers; patterns   |
| TC3 | `strict-casts`, `strict-inference` and `strict-raw-types` are true                                                                            | read `analyzer.language` in `analysis_options.yaml`                 | Dart 3.13 — customizing static analysis |
| TC4 | No `dynamic` in a public signature                                                                                                            | `grep -rnE "\bdynamic\b" lib` outside `lib/src` and generated files | Dart 3.13 — customizing static analysis |
| TC5 | `late` on a field has a comment naming what initialises it                                                                                    | `grep -rn "late " lib`                                              | Dart 3.13 — null safety                 |
| TC6 | No `!` on values that cross a package boundary; nullable results are handled where they arrive                                                | `grep -rnE "[a-zA-Z_)\]]!\." lib` and read each                     | Dart 3.13 — null safety                 |
| TC7 | Adding or tightening a class modifier on a published class is released as a breaking change                                                   | read the CHANGELOG and version for modifier changes                 | Dart 3.13 — class modifiers; versioning |

## Why each one

**TC1** because a public class with no modifier is open to extension and
implementation by every consumer. The day the author adds a method or a
private field that implementations must have, those consumers break — and
adding `final` then is itself a breaking change. The modifier is cheapest on
day one.

**TC2** is what `sealed` pays for: the compiler knows every subtype, so a
`switch` without a default fails to compile when a state is added.

**TC3** because Dart's defaults allow an implicit downcast from `dynamic`, which
is where JSON and platform-channel data enter; strict casts make each one
visible.
