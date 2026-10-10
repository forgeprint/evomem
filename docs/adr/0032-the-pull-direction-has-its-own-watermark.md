# ADR-0032: the pull direction has its own watermark

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0015

## Context

The work queued here was "a note deleted on the server is still on the
phone". Reading the pull side to fix it turned up four defects ahead of it,
each of which makes the deletion work pointless until it is fixed. None of
them is visible from the code; all four were found by writing a test that
asserted the behaviour the ADRs already claim.

### 1. Push and pull shared one watermark

`Pull` read and wrote `database.CursorNotes` — the row `pushNotes` uses.

Both are positions in an `(updated_at, id)` ordering, but over two different
tables: one counts local notes that have reached the mirror, the other counts
the mirror's notes that have reached here. So a pull set the push watermark
to a position in somebody else's ordering. Every local note behind that point
stopped being pending: `PendingNotes` would never return it again. The note is
still on the machine, it never reaches the mirror, and nothing anywhere says
the two sides disagree.

The same thing the other way: a push moved the pull watermark, so the notes
the mirror already held were skipped.

This is the worst kind of data loss — silent, permanent, and indistinguishable
from a store that is up to date.

### 2. A pull failed on the second run

`Pull` used `db.Create`, which fails on an identifier that already exists. The
first note it reached that this machine had pushed itself aborted the whole
pull. ADR-0015 says pull is "safe to run multiple times (idempotent)". It was
safe exactly once, on an empty store.

### 3. The cursor was truncated to a whole second

The remote hands the next position back as `RFC3339Nano`; `Pull` sent it on as
`RFC3339`. Against PostgreSQL, `updated_at > $1` then re-selected every note
written in that same second, on every pull, for ever. A pull that had caught
up tried to set the watermark to where it already was — which `SetCursor`
refuses, deliberately — so **the pull failed from then on, permanently, with
nothing wrong anywhere.** The fake remote in the tests parsed loosely enough
to hide this; only PostgreSQL shows it.

### 4. `evomem restore` could not run at all

`Restore` called `SetCursor(..., Cursor{})`. `SetCursor` refuses an empty
cursor by design, so the command failed on its second statement — before a
single note was read back. It also ended with the comment `// Update cursor
to end` above a bare `return nil`, so even had it got there, the next pull
would have walked the whole mirror again.

## Decision

### `CursorPulled` is its own row

A third watermark beside `notes` and `deletions`. `sync_state` is key/value
precisely so a new kind of watermark needs no migration, which is what that
design is for.

### Pulled notes are absorbed, not created

`database.Absorb` inserts or replaces by id, keeping the mirror's timestamps
— they are what the watermark on both sides is counted in, so rewriting them
to "now" would corrupt the ordering.

It **skips an incoming note that is not newer than the copy here.** Local
stays the authority for what this machine writes (ADR-0012): the mirror's
ordering says nothing about an edit made here and not yet pushed, so without
the check an older remote copy could land on top of a newer local one — and
then be pushed back over it. Last writer by `updated_at` wins, and a tie goes
to what is already here.

### `ResetCursor` is a separate method

`SetCursor` keeps refusing to move backwards; that refusal is worth keeping,
because a watermark that slipped back re-sends everything after it and the bug
is noticed by the bill. Restore is the one caller that means it, so it says so
by name.

### The cursor travels at full precision

`RFC3339Nano` both ways.

## Costs

- **A pulled note is pushed straight back.** Its `updated_at` is the mirror's,
  which is newer than this machine's push watermark, so the next push sends it
  up again. The mirror upserts, so nothing breaks and nothing changes — but it
  is a round trip of bandwidth per pulled note, every time. Fixing it means
  the push side knowing which rows arrived rather than originated, which is a
  column this schema does not have.
- **Last-writer-wins is a policy, not a merge.** Two devices editing the same
  note means the later `updated_at` wins outright and the other edit is gone.
  Nothing warns anybody. This is the same answer ADR-0019 gave for editing a
  pushed note, so it is at least consistent.
- **Clock skew is now load-bearing in a second place.** Two machines whose
  clocks disagree will order each other's edits by the wrong one.
- **The deletion question is still open and is not decided here.** See below.

## Still not decided: deletions coming the other way

The item this work started on is **not fixed**, and it should not be fixed
quietly, because doing it changes a decision that is recorded:

> there is no deletions table, because this sync is one-directional: the
> local file is the authority and a tombstone has nobody else to inform
> — `core/sync/postgres.go`, asserted by `TestRemoteSchemaShape`

`PushDeletions` deletes the mirror's row and leaves nothing behind. So there
is no tombstone in the mirror for a pull to read, and a note deleted on one
machine stays on every other one for ever. Worse than staying: the other
machine still has the note, its `updated_at` is ahead of its push watermark,
so **it pushes the deleted note back up and the deletion is undone.**

Making this work needs a tombstone table in the mirror, a retention rule for
it, and a pull that applies tombstones without recording them locally again.
That is a real change to what this sync is, and ADR-0012 called it
one-directional on purpose. It needs an answer, not a commit.
