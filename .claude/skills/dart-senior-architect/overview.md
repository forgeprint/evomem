# Dart Senior Software Architect

## What it changes

An agent asked to structure a Flutter app puts everything in one package with
folders called `domain`, `data` and `presentation`, adds `freezed` and commits
the generated files in one pull request and ignores them in the next, links
local packages with `path:` and `pubspec_overrides.yaml`, leaves every public
class open to extension, and — if there is native code — scatters `Pointer`
through the codebase and commits a prebuilt `.so`. With this expert:

- **Layers are packages in a pub workspace**, so `pubspec.yaml` decides what may
  import what; `lib/src` stays private; `dependency_validator` keeps the
  declarations honest; rules inside a package run as an analyzer plugin.
- **One workspace, one lockfile**, Melos on top of it if at all, and a
  published package's lower bounds actually tested.
- **Generated code has a written policy and a test**, and a published package
  ships its generated files — pub excludes what `.gitignore` excludes.
- **Public classes say what callers may do** with Dart 3 class modifiers, and
  closed sets of states are `sealed`.
- **FFI sits behind one package**, with `ffigen` bindings and build hooks
  instead of prebuilt binaries.
- **Publishing is checked before it happens**: dry run, semantic versions with
  the 0.x convention, `pana`, and a retraction plan.

Six checklists — package layering, pub workspace, code-generation policy, type
contracts, FFI boundary, pub publishing.

## What it fits

- Starting or restructuring a Flutter app or a pure-Dart codebase that is, or
  should become, more than one package.
- Adding code generation, native code or a package meant for pub.dev.
- Reviewing a Dart design, or a change that adds a dependency between layers.
- Dart 3.13 and Flutter 3.47. Workspaces need Dart 3.6+, analyzer plugins and
  build hooks 3.10+.

## What it does not fit

- **Building the app inside the structure** — screens, rebuild discipline,
  state management, routing, device storage, widget and golden tests. That is
  `flutter-mobile-engineer`, proposed separately. This expert decides where
  code lives; that one decides how a screen is written.
- **A single small package or script.** One package has no graph to hold; the
  workspace and layering rows will not apply, and this expert says so.
- **Dart on the server as a framework choice** — routing, middleware and
  hosting are not covered; the package, type and publishing rows still apply.
- **Application security**: [`security-reviewer`](../security-reviewer/SKILL.md).
- **CI and release automation**: [`devops-platform-engineer`](../devops-platform-engineer/SKILL.md).

## How it differs from its neighbour

| Question                                                              | Expert                    |
| --------------------------------------------------------------------- | ------------------------- |
| Which packages, what is generated, where FFI lives, what is published | `dart-senior-architect`   |
| How this screen, route, stored value or test is written               | `flutter-mobile-engineer` |

## Pros and cons

**In its favour:** its strongest rule costs nothing to run — a dependency that
is not in `pubspec.yaml` does not resolve — and the rest are commands with exit
codes: `dart analyze`, `dependency_validator`, `build_verify`, `dart pub
publish --dry-run`, `dart pub downgrade`.

**Against it:** splitting an app into packages is real work and is overhead for
a small app. Some tools it names are young: analyzer plugins arrived in Dart
3.10, and `import_lint` is a small package, so the expert names the
alternative. It is `provenance: generated`: drafted by a tool from the
2026-09-24 research, not written from someone's practice.

**What `agents: [claude-code]` rests on.** Claude Code ran the checklist greps
over this repository, which has no Dart code: they are well-formed and returned
nothing. It has not been run against a real Dart codebase. If it changes
nothing about what your agent does on one, say so in an issue.
