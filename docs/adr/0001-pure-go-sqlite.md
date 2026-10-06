# ADR-0001: SQLite through a pure-Go driver

**Status:** accepted · **Date:** 2026-10-06

## Context

Evomem's core is a single binary that a user installs and runs locally. The
store is SQLite. The obvious driver, `mattn/go-sqlite3`, binds the C library
through cgo.

## Decision

Use `modernc.org/sqlite`, SQLite transpiled to Go, and build with
`CGO_ENABLED=0`.

## Consequences

- One static binary per platform, with no libc to match and no C toolchain
  needed to build it. `scripts/crosscheck.sh` compiles five targets from one
  machine to keep that true.
- A write is measurably slower than through the C library. For a note store
  whose busiest writer is a chat webhook this is not the constraint.
- The binary is about 7 MB, nearly all of it the transpiled SQLite.
- The driver's FTS5 support is what the search depends on, so an upgrade is
  checked against `shared/database` rather than taken on trust.
