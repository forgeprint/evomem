# ADR-0012: Sync is one-directional, and the cursor only moves after a push

**Status:** accepted · **Date:** 2026-10-06

## Context

The local SQLite file is where notes are written — by `evomem add`, by the
adapters, by Apple Shortcuts. The remote is a copy. Phase 5 asks for "delta
synchronisation" without saying in which direction.

Two-directional sync would need per-field conflict resolution and a clock
that two machines agree on. Nothing in the product asks for it: a note is
written once, in one place, and rarely edited.

## Decision

The local file is the authority and the remote is a mirror. Changes go up and
nothing comes down.

**What changed is a watermark, not a flag.** `sync_state` holds a
`(updated_at, id)` pair per stream. Pending is everything past it, in that
order. The pair rather than the timestamp alone, because two notes written in
the same millisecond are otherwise indistinguishable and a cursor that can
only say "this millisecond" has to either re-send one or skip it. The id
breaks the tie, and being a ULID it breaks it in creation order.

No `synced` column on `notes`, so an edit needs no extra write: moving
`updated_at` past the cursor is what makes a note pending again.

**A delete leaves a tombstone.** Once the row is gone, absent is
indistinguishable from never written, and the cloud copy would outlive the
local one forever. The tombstone is written in the same transaction as the
delete.

**Archiving leaves none.** A tombstone means "the user removed this" and the
far end acts on it by deleting the cloud copy — which is the copy archiving
relies on. The two look identical in SQL and mean opposite things, which is
why `Delete` writes the tombstone explicitly rather than a trigger doing it
for both.

**The cursor moves after the push, per batch.** Not before, and not once at
the end.

## Consequences

- A failed push loses nothing: the cursor did not move, so the same batch is
  pending next pass. `TestFailedPushKeepsTheCursor` holds this.
- Pushes must therefore be idempotent, which the `Remote` interface says in
  as many words and the upsert implements. A retried batch does not duplicate.
- The upsert also refuses to apply an older `updated_at` over a newer one, so
  an out-of-order delivery cannot undo an edit.
- A run interrupted halfway through a backlog keeps what it managed, because
  the cursor moved per batch.
- `SetCursor` refuses to move backwards. Re-sending a note is harmless, but a
  cursor that went back would re-send everything after it on every pass —
  a bug noticed by the bill rather than by a test.
- **Archiving requires a sync.** A note past the cursor exists only locally,
  so `Archive` holds it back; `IncludeUnsynced` is the explicit way to say
  the data may be lost. A store that was never synced archives nothing and
  says so, which is why `ArchiveResult.Skipped` exists.
- Being offline is a state, not a failure: `Ping` failing gives
  `Stats.Offline` and no error, the worker backs off, and the next pass
  carries what this one could not.
- What this does not give: a second machine cannot pull the notes down, and
  restoring from the remote is not implemented. Both need their own decision,
  and the mirror's schema is deliberately plain enough to make either
  possible.
