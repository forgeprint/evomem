# ADR-0033: the mirror keeps tombstones

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0012, ADR-0032

## Context

ADR-0032 fixed the pull direction and left one thing open, on purpose,
because closing it changes a recorded decision:

> there is no deletions table, because this sync is one-directional: the
> local file is the authority and a tombstone has nobody else to inform
> — `core/sync/postgres.go`, asserted by `TestRemoteSchemaShape`

That reason was true when it was written. **ADR-0015 made it false.** Once
`evomem pull` exists, a second machine reads from the mirror, and a deletion
on the first has somebody to inform.

What actually happened without it:

1. A deletes a note. `PushDeletions` removes the mirror's row and leaves
   nothing behind.
2. B still has the note. Nothing it can read says otherwise, so it keeps it
   for ever — the item this work started from.
3. Worse: the note's `updated_at` is ahead of B's own push watermark, so B's
   next push **sends it back up**, and the mirror takes it. The deletion is
   undone by a machine whose only fault is being out of date, and nothing
   anywhere reports it.

Point 3 is the one that settles it. Without a tombstone, deletion is not
merely slow to propagate; it does not hold.

The user was asked and chose to add the table.

## Decision

### 1. `evomem_deletions` in the mirror

`(id PRIMARY KEY, project_id, deleted_at TIMESTAMPTZ)`, with an index on
`(deleted_at, id)` — the pair, because two deletions in the same instant are
otherwise indistinguishable and a watermark that can only say "this instant"
has to either repeat one or skip it.

`PushDeletions` writes the tombstone **in the same transaction** as the
removal, for the reason `database.Delete` gives locally: once the row is gone
there is nothing left to tell the far end it went.

### 2. The mirror refuses a note it holds a tombstone for

`insertNoteUnlessDeleted` guards the push with
`WHERE NOT EXISTS (SELECT 1 FROM evomem_deletions WHERE id = $1)`.

A client-side check could not do this: the machine pushing the note is
exactly the machine that has not heard about the deletion. The mirror is the
only place that knows, so it is the only place that can refuse.

### 3. A pull applies tombstones with `Forget`, not `Delete`

`Delete` writes a local tombstone, because a deletion made here has to
travel. One that came the other way must not be recorded again, or it would
be pushed straight back to the mirror that reported it, on every pass, for
ever. `Forget` removes the row and the recording and leaves nothing behind.

An identifier that is not here is not an error: the mirror tells every
machine, including the ones that never saw the note.

### 4. Tombstones are pulled after notes, and on every pass

After, because a note written and then deleted arrives in both batches and
the deletion has to be the later word.

On every pass including one that found no new notes — which was a bug for
about ten minutes: `Pull` returned early when there was nothing to read, and
a pass with nothing but a deletion waiting is precisely the common case.

### 5. `CursorPulledDeletions`, its own watermark

The fourth row in `sync_state`, for the reason ADR-0032 gives: it counts in
`(deleted_at, id)` over the mirror's table and shares an ordering with
nothing else.

## Costs

- **The sync is no longer one-directional, and ADR-0012 says it is.** That
  claim is now about authorship — notes are written locally and pushed —
  rather than about traffic. Deletions travel both ways and notes come down
  on a pull.
- **The mirror's tombstones are never removed.** They have to outlive every
  machine that might still be holding the note, and nothing tracks which
  machines exist (ADR-0030: there are no accounts). For one person's notes
  the table grows by one short row per deletion, which is not worth a
  retention rule yet. If it ever is, the rule needs to know what "every
  machine has heard" means, and nothing does.
- **A deleted identifier can never be reused.** It is a ULID, so this costs
  nothing in practice — but `evomem restore` followed by a re-import of the
  same identifiers would find them refused, silently.
- **The refusal is silent.** `PushNotes` drops the row and reports success,
  the way the existing older-version guard does. A machine pushing a deleted
  note is told nothing. Making it loud means counting affected rows per
  statement and deciding what the caller should do about it, which is a
  bigger change than this.
- **Clock skew decides the order.** A tombstone from a machine whose clock is
  behind can sort before the note that a machine with a correct clock wrote.

## Verified

Against real PostgreSQL (a scratch database, dropped afterwards), 2026-10-10:
a deletion made on one machine reaches another; the pull settles instead of
repeating; and a push of the deleted note is refused — removing the guard
makes that last assertion fail, which is how it was checked rather than
assumed.

## Scope, stated plainly

This record is about **`core/sync`**: a local Go store and the PostgreSQL
mirror. "Every machine" means every machine on that path.

**The Flutter app is not on it.** It talks HTTP to `core/api` and only ever
pushes — `sync_service.dart` has no pull of any kind. So a note deleted on the
server is still on the phone, exactly as before, and nothing here changed
that.

Giving the phone a pull is a separate decision, and a recorded one stands in
front of it:

> A view, not a sync mechanism (ADR-0024): a client asking what is here is
> not the same as a client pulling it into a store of its own, and calling it
> a pull would reopen the question ADR-0012 closed.
> — `core/api/read.go`, above `handleListNotes`

It also needs something `GET /notes` does not have: a cursor, and a way to
learn about tombstones. Neither is written.
