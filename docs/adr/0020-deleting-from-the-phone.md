# ADR-0020: A deletion made on the phone travels as a tombstone

**Status:** accepted · **Date:** 2026-10-09

## Context

Deleting a note on the phone removes the local row and nothing else. The
mirror keeps it for ever, and `_pushDeletions` says so in a comment rather
than doing anything:

```dart
// For now, we don't track deletions locally in the mobile app
return SyncResult.success(notesPushed: 0, deletionsPushed: 0);
```

ADR-0019 named this and left it out: "Deleting from the phone. Needs a local
deletions table and a route of its own."

Deletion is the one operation where doing nothing is **worse** than doing
nothing elsewhere. A note that fails to sync is a note the mirror has not
heard about yet. A deletion that fails to sync is content the person
deliberately removed, still readable by every agent the mirror feeds — and
nothing anywhere says the two copies disagree.

### What already exists

- The phone's schema has a `deletions` table from v2 — `(id, project_id,
  deleted_at)` — which nothing has ever written to.
- `database.Delete` on the server writes a tombstone in the same transaction
  as the delete, deliberately: "once the row is gone there is nothing left to
  tell the far end it went". It also removes the note's recording (ADR-0018).
- `core/sync` already pushes those tombstones to PostgreSQL.

So the chain from the server to the cloud is built. What is missing is the
first hop, phone to server.

## Decision

### 1. `DELETE /notes/{id}`, the third write route on one token

Registered only with `EVOMEM_API_TOKEN`, bearer-authenticated, addressed by
the `remote_id` the phone kept (ADR-0018). It calls `database.Delete`, so the
server writes its own tombstone and the deletion carries on to PostgreSQL
through machinery that already exists.

An unknown id answers **404**, as `database.Delete` already reports:
"a caller deleting by an identifier it was given wants to know the identifier
was wrong." The *phone* treats that 404 as done — the mirror does not have
the note, which is what the phone was asking for — in the same shape as the
`PUT` 404 of ADR-0019.

### 2. Only a note the server has ever seen leaves a tombstone

A note with no `remote_id` was never accepted, so there is nothing to tell
anybody about. It is deleted locally and no tombstone is written. A tombstone
for a note the mirror never had would be a request the server could only
answer 404 to.

### 3. The local tombstone goes when the server has taken it

`deletions` is a queue, not a log. A row is removed once the server answers
200 or 404; a row that fails any other way stays and is retried by the next
push. Keeping them for ever would mean every sync re-sends every deletion the
phone has ever made.

This is the opposite of the server's own tombstones, which are kept until the
*cloud* cursor passes them and then archived (ADR-0012). The difference is
that the server has to keep telling PostgreSQL, while the phone has exactly
one listener.

### 4. Schema v6 adds `deletions.remote_id`

The table's `id` is the phone's own identifier, which the server has never
heard of. The remote id is the only thing a `DELETE` can be addressed with,
and it is kept in a column of its own rather than overloading `id`, so a row
says plainly which identifier belongs to whom.

## Consequences

### Costs accepted

- **A deletion can be lost.** If the phone's local row is removed and the
  tombstone push then fails permanently — the app is uninstalled, the
  database is cleared — the mirror keeps the note and nobody is told. The
  window is one sync, and nothing closes it completely short of deleting on
  the server first and the phone second, which would make the phone unusable
  offline.
- **A fourth thing one token can do.** `/ingest`, `/ingest/audio`,
  `PUT /notes/{id}` and now a delete. A leaked token could already write and
  edit arbitrary notes; it can now remove them, which is the first
  irreversible thing it can do. The mitigation is the one ADR-0010 already
  chose — loopback by default, a tunnel the user puts up deliberately — not a
  second credential.
- **Last writer wins, still.** A note deleted on the phone and edited on the
  server ends up deleted, whichever happened first, because nothing compares
  the two clocks.

### Explicitly not decided here

- **Deleting on the server and having the phone notice.** `evomem pull` reads
  notes, not tombstones, so a note deleted on the server stays on the phone.
  That is the other direction and needs the pull side to read the deletions
  table.
- **Undo.** Neither side keeps the content of a deleted note, so there is
  nothing to restore from. `evomem restore` pulls from the mirror, which by
  then has also deleted it.
