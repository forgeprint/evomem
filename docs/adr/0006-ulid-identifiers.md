# ADR-0006: ULID primary keys, written here

**Status:** accepted · **Date:** 2026-10-06

## Context

A note needs an identifier that can be minted offline, on a phone, by an
adapter, without asking the database — so not an autoincrement. The default
choice is a UUID v4, which is random: in a B-tree index, consecutive inserts
land in unrelated pages and split them all over the index.

## Decision

Use ULIDs: 48 bits of millisecond timestamp then 80 bits of randomness, in
26 characters of Crockford base32. Written in `shared/models/ulid.go` rather
than imported, under ADR-0002.

Stored and compared upper case. `DB` normalises on the way in and on lookup,
because SQLite compares `TEXT` byte by byte and would otherwise treat two
spellings of one identifier as two rows.

## Consequences

- The text form sorts in creation order, so inserts append to the right-hand
  edge of the index instead of splitting pages across it.
- `ORDER BY created_at DESC, id DESC` breaks a same-millisecond tie in
  creation order rather than arbitrarily.
- Identifiers within a millisecond are ordered too: the random half is
  incremented rather than redrawn, which also survives a clock that steps
  backwards (`TestNewULIDSurvivesBackwardsClock`).
- A ULID leaks its creation time to anyone holding it. For a local note store
  that is already visible in `created_at`.
- About 150 lines to maintain, with its own tests, rather than a dependency.
