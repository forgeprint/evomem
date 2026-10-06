# ADR-0004: Two pools, one writer

**Status:** accepted · **Date:** 2026-10-06

## Context

SQLite in WAL mode allows many readers alongside one writer, and a second
writer gets `SQLITE_BUSY`. Evomem writes from several places at once: a
Telegram webhook, an Apple Shortcut, an HTTP request, an MCP client reading
while any of them writes. Left to `database/sql`'s default pool, two
concurrent writes are a race whose loser surfaces as an error to a user.

## Decision

`database.DB` holds two pools over the same file:

- the write pool, capped at one connection (`SetMaxOpenConns(1)`) and opened
  with `_txlock=immediate`;
- the read pool, sized to the machine.

Both set `journal_mode=WAL`, `busy_timeout=5000`, `synchronous=NORMAL`.

## Consequences

- Concurrent writes queue inside the process instead of failing outside it.
  `TestConcurrentWritesDoNotCollide` is what would catch the cap's removal.
- Writes are serialised, so a long write transaction delays every other write.
  No write in this layer holds a transaction open across anything slow.
- `_txlock=immediate` takes the lock at `BEGIN`. Without it SQLite starts
  deferred and upgrades on first write, and an upgrade that loses cannot be
  retried safely because the transaction has already read.
- `synchronous=NORMAL` means a crash can lose the last commits but cannot
  corrupt the file. That trade buys an order of magnitude on ingestion.
- `busy_timeout` still matters: it covers another *process* holding the file,
  which the in-process cap says nothing about.
