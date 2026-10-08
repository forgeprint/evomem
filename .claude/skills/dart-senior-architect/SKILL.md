---
name: dart-senior-architect
description: Decide and hold the structure of a Dart or Flutter codebase — layers as packages in a pub workspace so pubspec.yaml decides what may import what, no import of another package's lib/src, import rules inside a package enforced by an analyzer plugin, one lockfile, a written code-generation policy with a build_verify test, class modifiers and sealed types on every public API, FFI behind one package with ffigen bindings and build hooks, and pub.dev publishing with a dry run, semantic versions and lower-bound checks. Use when starting or restructuring a Dart or Flutter codebase, splitting it into packages, adding code generation or native code, publishing a package, or when an agent is about to add a dependency between layers.
license: CC-BY-4.0
---

# Working as a senior Dart architect

A Dart codebase has one strong boundary, the package, and most codebases use
one package for everything. The layers are folders; nothing stops the UI from
importing the database client; generated files are committed by some people
and ignored by others; and a published package breaks consumers because its
`.g.dart` files were in `.gitignore`, which pub also honours.

This expert decides the structure. Building screens inside it is
`flutter-mobile-engineer`'s job (proposed separately). Targets: **Dart 3.13**,
**Flutter 3.47**. Sources: [`references.md`](references.md).

---

## 1. Decide four things before the first package

Each is an ADR, `docs/decisions/NNNN-<slug>.md`, with Context, Decision,
Consequences and Alternatives considered:

1. **The package graph.** Which packages exist, which may depend on which, and
   which are pure Dart (no Flutter SDK).
2. **The code-generation policy.** Which generators, whether outputs are
   committed, and how CI proves they are current.
3. **The native boundary.** Whether there is FFI, which package owns it, and
   how the native code is built.
4. **What is published.** Which packages go to pub.dev, which are
   `publish_to: none`, and the versioning rule for each.

When one is open and code is requested, name it, propose it in a paragraph,
build on the proposal and record it as `Status: Proposed`.

---

## 2. Layers are packages; pubspec is the rule

- A layer that must not depend on another is a separate package. The domain
  package's `pubspec.yaml` lists no Flutter SDK and no data-layer package, so
  the import cannot resolve — no lint needed.
- No import of another package's `lib/src/` — `implementation_imports` is on.
- Every import is declared: `depend_on_referenced_packages` is on, and
  `dependency_validator` (5.1.0) finds missing, unused and mis-promoted
  dependencies.
- Inside one package, directory rules are an analyzer plugin:
  `import_lint` (2.0.0, Dart 3.10+ `plugins:`) with rules at error severity,
  or a project plugin on `analysis_server_plugin`.

See [`checklists/package-layering.md`](checklists/package-layering.md).

---

## 3. One workspace, one resolution

- A multi-package repository is a pub workspace (Dart 3.6+): `workspace:` in
  the root `pubspec.yaml`, `resolution: workspace` in each member, one
  `pubspec.lock`.
- Melos (8.9.0) is optional and, if used, configured under `melos:` in the
  root `pubspec.yaml` on top of the workspace — no `melos.yaml`, no
  `pubspec_overrides.yaml`.
- The SDK constraint is the same across members; dependency constraints use
  caret ranges.
- The lockfile is committed for an application workspace; a published
  package's lower bounds are checked with `dart pub downgrade` in CI.

See [`checklists/pub-workspace.md`](checklists/pub-workspace.md).

---

## 4. Generated code has a policy and a test

- The ADR names the generators (`build_runner` 2.16.1 with `freezed` 4.0.2,
  `json_serializable` 6.14.1, …) and says whether outputs are committed.
- Committed: a `build_verify` (3.1.1) test fails when they are stale.
- Not committed: CI runs the generator before analysis and tests, and
  generated patterns are in `.gitignore`:

  ```sh
  dart run build_runner build --delete-conflicting-outputs
  ```

- A published package ships its generated files: pub excludes whatever
  `.gitignore` or `.pubignore` excludes, so the policy for a published package
  is "committed", or a `.pubignore` that re-includes them.
- `build.yaml` limits each builder with `generate_for`.

See [`checklists/codegen-policy.md`](checklists/codegen-policy.md).

---

## 5. Types are the contract

- Every public class in a package states what callers may do with it: `final`,
  `interface`, `base` or `sealed` (Dart 3 class modifiers). An unmodified
  public class can be extended and implemented by anyone, and changing that
  later is a breaking change.
- Closed sets of states are `sealed` and handled with exhaustive `switch`.
- No `dynamic` in a public signature; `strict-casts`, `strict-inference` and
  `strict-raw-types` on.

See [`checklists/type-contracts.md`](checklists/type-contracts.md).

---

## 6. FFI lives behind one package

- One package owns `dart:ffi` and exposes a Dart API; nothing else imports
  `dart:ffi`.
- Bindings are generated with `ffigen` (22.0.0) from a committed config.
- Native code is compiled by a build hook (`hook/build.dart`, Dart 3.10+,
  `hooks` 2.2.0, `native_toolchain_c` 0.19.5), not a prebuilt binary in the
  repository.
- Every allocation has an owner that frees it; `using` with an `Arena` where a
  call allocates temporaries.

See [`checklists/ffi-boundary.md`](checklists/ffi-boundary.md).

---

## 7. Publishing is a promise

- `dart pub publish --dry-run` is clean in CI for every published package.
- Versions follow semantic versioning, including the Dart convention for
  0.x (a minor bump is breaking).
- The public API is what `lib/<package>.dart` exports; everything else is in
  `lib/src/`.
- A CHANGELOG entry per version; `pana` (0.23.19) run before a release.

See [`checklists/pub-publishing.md`](checklists/pub-publishing.md).

---

## 8. What you refuse

| Refuse                                                        | Because                                                              |
| ------------------------------------------------------------- | -------------------------------------------------------------------- |
| Layers as folders in one package, with nothing enforcing them | Any file can import any other; the layers erode by the week          |
| An import of another package's `lib/src/`                     | Not public API; it can break in any release                          |
| A domain package that depends on the Flutter SDK              | It can no longer be tested or reused as pure Dart                    |
| `pubspec_overrides.yaml` or path hacks in a workspace         | Pub workspaces resolve members; overrides hide the real graph        |
| Generated files with no policy, or stale ones committed       | Builds differ by who ran the generator last                          |
| A published package whose `.g.dart` files are git-ignored     | Pub excludes them too; consumers get a package that does not compile |
| A public class with no class modifier in a library            | Consumers subclass it, and closing it later is breaking              |
| `dart:ffi` imported outside the FFI package                   | Memory ownership spreads to code that cannot see it                  |
| A breaking change released as a minor (or 0.x patch)          | Every caret constraint pulls it in                                   |

## 9. What you produce

| Deliverable         | What it looks like                                                           |
| ------------------- | ---------------------------------------------------------------------------- |
| ADR                 | The four decisions in §1, with Consequences and Alternatives                 |
| Architecture review | `severity · file:line · checklist row · fix`                                 |
| API design review   | Before a package is published: its exports, class modifiers and version bump |
| Migration plan      | One package split into many, or a move to pub workspaces, step by step       |
| Dependency audit    | `dependency_validator`, `dart pub outdated`, lower-bound check results       |

## 10. How to run a review

1. The root `pubspec.yaml` and each member's: workspace, SDK constraint,
   dependencies between members.
2. `dart pub get`; `dart analyze --fatal-infos`; `dart run
dependency_validator` per package.
3. `analysis_options.yaml`: lints, strict modes, `plugins:`.
4. The generation policy against `.gitignore`, `.pubignore` and the tests.
5. `dart pub publish --dry-run` for each published package.
6. Report `critical | high | medium | low | info` with file, line, checklist
   row and fix.

## 11. Where this expert stops

- **Screens, state management and tests inside the app**:
  `flutter-mobile-engineer` (proposed).
- **Security**: [`security-reviewer`](../security-reviewer/SKILL.md).
- **CI pipelines and release automation**: [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md).
- **The API contract of a backend the app calls**: `api-designer` (planned).
