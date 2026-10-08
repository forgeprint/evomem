# FFI boundary

`dart:ffi` calls native code directly. Memory is manual, types are unchecked
across the boundary, and a crash takes down the process. The architecture keeps
all of that inside one package with a Dart API on top.

| #   | Check                                                                                           | How                                                                                  | Source                                                    |
| --- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | --------------------------------------------------------- |
| FB1 | One package owns `dart:ffi`; no other package imports it                                        | `grep -rln "import 'dart:ffi'" --include=*.dart .` lists files of one package only   | Dart 3.13 — C interop                                     |
| FB2 | That package exposes a Dart API with no `Pointer` or `ffi` types in its public signatures       | read `lib/<package>.dart` exports; `grep -rn "Pointer<" lib` outside `lib/src`       | Dart 3.13 — C interop; package layout                     |
| FB3 | Bindings are generated with `ffigen` from a committed config, not written by hand               | `ffigen` config in `pubspec.yaml` or `ffigen.yaml`; bindings regenerate with no diff | ffigen 22.0.0                                             |
| FB4 | Native code is compiled by a build hook (`hook/build.dart`), not committed as a prebuilt binary | `ls hook/build.dart`; `git ls-files '*.so' '*.dylib' '*.dll'` returns nothing        | Dart 3.13 — hooks; hooks 2.2.0; native_toolchain_c 0.19.5 |
| FB5 | Every allocation has one owner that frees it; temporaries use `using` with an `Arena`           | `grep -rn "calloc\|malloc" lib`; each has a matching `free` or sits in `using`       | ffi 2.2.0                                                 |
| FB6 | Long native calls do not run on the UI isolate                                                  | read call sites in Flutter code for synchronous FFI calls on user actions            | Flutter 3.47 — concurrency and isolates                   |
| FB7 | The native library version and the ABI it is built against are recorded                         | the ADR or the hook's configuration names the upstream version                       | Dart 3.13 — hooks                                         |

## Why each one

**FB1 and FB2** are the boundary itself. Memory ownership and pointer lifetimes
are hard to reason about locally and impossible to reason about when
`Pointer<Utf8>` travels through three packages.

**FB4** because a prebuilt binary in the repository is a file nobody can
review, built on a machine nobody can reproduce, for the platforms somebody
remembered. Build hooks, stable since Dart 3.10, compile the native code as
part of `dart run` and `flutter build`.

**FB5** because a leak across FFI does not show up in Dart's heap tools; the
process grows until the operating system stops it.
