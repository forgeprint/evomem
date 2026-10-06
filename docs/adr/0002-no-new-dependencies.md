# ADR-0002: The standard library and one dependency

**Status:** accepted · **Date:** 2026-10-06

## Context

Evomem is installed and trusted by people who will not read its dependency
tree. Every module in that tree is something that can change under the
project, in tests as much as in the binary.

## Decision

The Go side depends on the standard library and `modernc.org/sqlite`, and
nothing else — including for tests. Dependencies are vendored, so a build
needs no network and no module cache.

Anything that looks like it needs a new dependency is raised with the user
first. If it is agreed, it comes with its own ADR and is vendored.

## Consequences

- Small utilities are written here instead of imported. `shared/models/ulid.go`
  is the first: about 150 lines rather than a module.
- `vendor/` is 136 MB on disk and is committed. It is exempt from line-ending
  normalisation in `.gitattributes` so the tree stays byte-identical to what
  upstream published, and `scripts/test.sh` excludes it from the format check.
  Never run `gofmt -w .` from the repository root — it rewrites vendor and no
  check will catch it.
- No test framework, so tests are table-driven standard-library tests.
- The Flutter side has its own ecosystem and is not covered by this.
