# Syncing and archiving

The local SQLite file is where notes are written. The remote PostgreSQL copy
is a mirror: changes go up, nothing comes down. See
[ADR-0012](adr/0012-one-way-sync.md).

## Setting it up

The connection string carries a password, so it comes from the environment
rather than a flag — a flag is visible in `ps`.

```sh
export EVOMEM_POSTGRES_DSN='postgres://user:password@host:5432/evomem?sslmode=require'

evomem sync -init-remote   # creates evomem_notes and its indexes, then stops
evomem sync -once          # one pass
evomem sync                # every 5 minutes until stopped
```

`-init-remote` is separate on purpose: running DDL against someone's database
because a process started is not a sync client's decision. Until it has been
run, `evomem sync` says so rather than failing a push every five minutes.

## What a pass does

Notes first, then deletions, in batches of 200.

The watermark is a `(updated_at, id)` pair per stream, kept in `sync_state`.
Everything past it is pending; the pair rather than the timestamp alone,
because two notes written in the same millisecond would otherwise be
indistinguishable. There is no `synced` column on `notes`: an edit moves
`updated_at` past the cursor, which is what makes the note pending again.

The cursor moves **after** each batch succeeds. A failed push loses nothing —
the same batch is pending next pass — and a run interrupted halfway through a
backlog keeps what it managed.

Being offline is a state, not an error. `evomem sync -once` prints *the remote
is unreachable; nothing was sent* and exits 0. The loop backs off: the
interval doubled per consecutive failure, capped at 30 minutes, with up to a
quarter added at random so several machines do not arrive together after a
shared outage.

```sh
evomem sync-status
```

```text
store /Users/you/.evomem/evomem.db
notes      0 pending, cursor 2026-10-06T06:55:45Z 01M47ZYCCFKNCK78AT01GBK6ZN
deletions  1 pending, cursor (nothing synced)
```

It reports the push side. The pull's own watermarks are in the same table
under `pulled` and `pulled_deletions`; nothing prints them yet.

## Deleting

```sh
evomem delete 01M47ZYCCFKNCK78AT01GBK6ZN
```

This leaves a tombstone, and the tombstone is the point: once the row is
gone, absent is indistinguishable from never written, and the cloud copy would
outlive the local one forever. It is written in the same transaction as the
delete and travels on the next pass.

## Archiving

```sh
evomem archive -dry-run              # say what would go
evomem archive                       # notes older than 6 months
evomem archive -months 12 -vacuum    # and return the space to the disk
```

**Archiving is not deleting.** It thins a local file whose contents exist in
the mirror, so a note past the sync cursor is held back — that note exists
only here. A store that was never synced archives nothing and says so:

```text
cutoff 2026-04-06T07:02:11Z
removed 0 note(s), 0 tombstone(s)
kept 2 note(s) that are old enough but have not been synced;
  they exist only in this file. -include-unsynced removes them anyway.
```

`-include-unsynced` is the explicit way to say the data may be lost.

Archiving leaves **no** tombstones. A tombstone would tell the far end to
delete the cloud copy — the copy archiving relies on.

Synced tombstones older than the cutoff are cleared too, and so are proposals
a person decided on before it. Otherwise those two tables are the only things
in the store that just grow. A **pending** proposal is never archived, whatever
its age: nobody has looked at it yet.

`-vacuum` rewrites the file, which is what actually returns the space; a
delete alone leaves free pages inside it. This is where the explicit
`rowid INTEGER PRIMARY KEY` from [ADR-0005](adr/0005-external-content-fts.md)
earns itself: `VACUUM` may renumber an implicit rowid, and the full-text index
is joined to notes by rowid.

## Testing against a real PostgreSQL

The transport's tests are skipped unless a database is pointed at, so
`go test ./...` needs none:

```sh
docker run -d --rm --name evomem-pg -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=evomem_test -p 55432:5432 postgres:17-alpine

EVOMEM_TEST_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55432/evomem_test?sslmode=disable' \
  go test ./core/sync/
```

`scripts/ci.sh` does not run them: it has to stay runnable offline on any
machine.

## The mirror's schema

```sql
CREATE TABLE evomem_notes (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    content     TEXT NOT NULL,
    source_type TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

```sql
CREATE TABLE evomem_deletions (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL
);
```

Not a copy of the local schema. There is no FTS table — PostgreSQL has its own
full-text machinery and picking a configuration for it is a decision nobody
has asked for. `synced_at` is the remote's own column: it answers "when did
this machine last reach us".

There **is** a deletions table, and there did not use to be. The reason given
was that a tombstone on a mirror has nobody to inform; `evomem pull` made that
false. Without it a note deleted on one machine stayed on every other one for
ever — and got pushed back up, undoing the deletion. A push is now guarded by
`NOT EXISTS (SELECT 1 FROM evomem_deletions WHERE id = $1)`, because the
machine pushing the note is exactly the machine that has not heard about the
deletion: the mirror is the only place that knows, so it is the only place
that can refuse. → ADR-0033

Tombstones are never removed. A retention rule would have to know when every
machine has heard, and with no account model (ADR-0030) nothing knows that.

The `metadata` column carries everything the adapters wrote, including the
`tainted` mark, so the far end can tell third-party text from the user's own.

## Reading back

```sh
evomem pull      # what the mirror has that this machine does not
evomem restore   # throw the local copy away and read the mirror whole
```

`pull` walks the mirror's notes and then its tombstones, from its own
watermarks — `pulled` and `pulled_deletions` in `sync_state`, separate rows
from the push's. They count in different orderings over different tables, and
sharing one meant a pull set the push watermark to a position in the mirror's
ordering: every local note behind it quietly stopped being pending, still here
and never sent. → ADR-0032

A pulled note is written as the mirror holds it, keeping the mirror's
timestamps, and **an incoming note that is not newer than the copy here is
skipped** — so a pull cannot land on top of an edit made here and not yet
pushed. Local stays the authority for what this machine writes.

A pulled tombstone removes the note and records nothing: a tombstone written
here would be pushed straight back to the mirror that reported it, on every
pass, for ever.

`restore` empties the local notes and reads the mirror whole. It is the one
caller that resets a watermark, which `SetCursor` otherwise refuses.

## What this does not do

**No merge.** Where two machines disagree about one note, the later
`updated_at` wins outright and the other version is gone, with nothing warning
anybody (ADR-0019, ADR-0032). The clock is load-bearing: two machines whose
clocks disagree order each other's edits by the wrong one.

**No per-project or per-age pull.** `pull` takes everything the mirror has
that this machine does not.

**Notes are still written on a machine, not on the mirror.** That is what
ADR-0012 means by one-directional now; traffic goes both ways.
