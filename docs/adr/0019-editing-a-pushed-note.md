# ADR-0019: An edit reaches the mirror through PUT /notes/{id}

**Status:** accepted · **Date:** 2026-10-09

## Context

ADR-0018 made the phone keep the identifier `/ingest` returns and stop posting
a note it had already pushed. That closed two defects — a recording could be
attached, and a half-failed batch stopped duplicating notes — and opened one,
which that record names as a deliberate trade:

> an edited note no longer reaches the server at all, since /ingest cannot
> update. Stale instead of duplicated.

Stale is the better of those two, and still wrong. A person edits a note on
the phone, watches it change, and the mirror keeps the first wording for ever
with nothing anywhere saying the two disagree. Before ADR-0018 the edit at
least arrived, as a second note; now it arrives not at all.

`/ingest` cannot carry an edit. It mints an identifier and calls `Create`, so
posting an edited note makes a new one. That is also what keeps it simple and
why it has two body formats for Apple Shortcuts' sake: it is an *arrival*
endpoint.

### The phone already re-reads an edited note

`getNotesAfterCursor` pages on `(updated_at, id)` ascending. Editing a note
moves its `updated_at` forward, so it crosses the cursor again and the next
push reads it. The only thing stopping the edit was ADR-0018's skip of a note
that has a `remote_id`.

**That is not quite enough, and the first draft of this record said it was.**
The cursor brings an edited note back, but it also brings back every note in a
batch that failed half way, because the cursor only advances after a whole
batch succeeds. With nothing but `remote_id`, the phone cannot tell a note
that was edited since it was pushed from one that was not, so a retry writes
both to the mirror. A test caught it: a retried two-note batch made two
requests where one was expected.

So there is one new column after all — see decision 5.

## Decision

### 1. A second endpoint, not an upsert on the first

`PUT /notes/{id}` replaces the content and the metadata of a note the server
already has. The phone addresses it with the `remote_id` it kept.

The alternative was making `/ingest` upsert on a client-supplied identifier.
Rejected: it would let a sender choose identifiers, which is how one caller
overwrites another's note by naming it, and it would give the arrival endpoint
a third body shape. An edit is a different act from an arrival and says so in
the method.

### 2. It changes what a note says, never what it is

Content and metadata only — the same fields `database.Update` already writes,
for the reason that record gives: "The identifier, the project and the source
are what the note is, not what it currently says." A `PUT` that moved a note
to another project would be a different note wearing the same id.

`updated_at` is set by the server, not taken from the body. A clock the server
does not own has no business ordering its rows, and `updated_at` is half of
the sync cursor.

### 3. Same token, same posture as /ingest

Registered only with `EVOMEM_API_TOKEN`, bearer-authenticated, body bounded at
1 MiB like everything else in that package — audio is the one exception
(ADR-0018), and an edit is text. No token, no endpoint (ADR-0010).

An edit arrives under the user's own token, so the note is **not** marked
tainted, exactly as `/ingest` does not mark one.

### 4. A note the server no longer has is reported, not resurrected

`PUT` on an unknown id answers **404** and writes nothing. The phone records
nothing and moves on.

The alternative — fall back to `POST /ingest` and keep the new identifier —
would bring back a note somebody deleted on the server, which is the worst
outcome available: a delete that undoes itself the next time the phone syncs.
A 404 therefore ends that note's story on the mirror while leaving the local
copy alone.

This is the part of this decision most likely to be wrong, and it is written
down so that changing it is a decision rather than a patch.

### 5. The phone records what the mirror holds

Schema v5 adds `notes.remote_updated_at`: the note's own `updated_at` at the
moment the server accepted it. A note is edited-since-push when its
`updated_at` is later than that, and only then is it sent.

Without it a retried batch rewrites notes that did not change, which moves
their `updated_at` **on the mirror** — and that is the cursor `evomem pull`
gives another device, so a redundant write here becomes a redundant read
there.

An existing row gets an empty value, which reads as "pushed, and we do not
know what it said then". Those notes count as changed, so each gets one
redundant `PUT` on its next edit and is exact from then on. Guessing
"unchanged" instead would have left a genuinely edited note stale for ever,
which is the defect this record exists to fix.

## Consequences

### Costs accepted

- **An edit can still be lost, quietly.** A `PUT` that fails for a reason
  other than 404 fails the batch, so the cursor does not advance and the next
  run retries it — but only while the cursor is still behind that note. Once
  a later batch has carried the cursor past it, nothing brings it back until
  it is edited again. No retry queue exists.
- **One more column, and a migration.** `remote_updated_at` is the price of
  not rewriting the mirror on every retry, and it is a fact about this device
  rather than about the note, so it is the phone's schema that carries it and
  the two version numbers drift further apart.
- **Last writer wins, and the phone is the only writer.** Sync is one-way
  (ADR-0012): nothing reconciles an edit made on the server with one made on
  the phone, and `PUT` overwrites. `evomem pull` can already bring remote rows
  down, so a person who edits on both sides will lose one of the two edits
  without being told which.
- **A third endpoint to keep authenticated.** `/ingest`, `/ingest/audio` and
  now this one, all on one token. A leaked token already meant arbitrary
  writes; it now also means arbitrary edits of existing notes, which is a
  wider blast radius for the same credential.
- **The deletion half stays missing.** The phone has no local deletions table
  (`_pushDeletions` returns zero and says so), so a note deleted on the phone
  still lives on the mirror. This record does not fix that; it is named here
  so the gap is not mistaken for this decision's doing.

### Explicitly not decided here

- **Two-way sync or conflict resolution.** ADR-0012 chose one-way and that
  stands; this makes the one direction complete for edits, not bidirectional.
- **Deleting from the phone.** Needs a local deletions table and a route of
  its own.
