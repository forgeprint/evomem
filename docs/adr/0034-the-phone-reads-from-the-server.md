# ADR-0034: the phone reads from the server

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0012, ADR-0024

## Context

The app only ever pushed. `sync_service.dart` had no pull of any kind, so a
note deleted on the server stayed on the phone for good, and a note written
anywhere else never arrived. ADR-0033 fixed the same problem on the
`core/sync` path — a local Go store and the PostgreSQL mirror — and the phone
is not on that path at all; it talks HTTP to `core/api`.

Two recorded decisions stood in front of this and the user was asked:

> A view, not a sync mechanism (ADR-0024): a client asking what is here is
> not the same as a client pulling it into a store of its own, and calling it
> a pull would reopen the question ADR-0012 closed.
> — `core/api/read.go`, above `handleListNotes`

They chose to give the phone a pull.

## Decision

### 1. Two new endpoints, not a flag on `GET /notes`

`GET /sync/notes` and `GET /sync/deletions`, behind the same token as
everything else that reads.

`GET /notes` stays exactly what its comment says: a view. It is newest-first
with offset paging, and a client paging through a view while somebody writes
to it silently misses rows — which is the failure mode a sync must not have.
The pull endpoints order by the same `(updated_at, id)` pair the mirror sync
uses, which is what the index exists for.

The cursor comes back **even when nothing did**, so a client that has caught
up stores where it is rather than an empty string. Storing an empty string is
precisely how the mirror's own pull broke permanently (ADR-0032), and that
mistake was available here too.

Half a cursor is refused rather than guessed at: a time with no identifier can
only say "this instant", which has to either repeat a row or skip one.

### 2. The phone keeps its own two watermarks

`sync_cursor_pulled` and `sync_cursor_pulled_deletions`, beside the existing
`sync_cursor_notes`. Separate rows for the reason ADR-0032 gives at length:
they count in different orderings over different stores, and sharing one row
means a pull silently strands local notes that have never been sent.

### 3. `absorb` and `forgetByRemoteId` on the store port

`absorb` writes a note as the server holds it, keeping the server's
timestamps — they are what the cursor on both sides is counted in. It **skips
an incoming note that is not newer than the copy here**, so a pull cannot land
an older server copy on top of an edit made on this device and not yet pushed.

`forgetByRemoteId` removes a note the server says is gone **without recording
a local deletion**. Recording one would push it straight back to the server
that reported it, on every sync, for ever.

### 4. A pulled note does not echo back up

`/ingest` creates a note and returns a fresh identifier, so an echo would be a
duplicate rather than a no-op. What prevents it is that `absorb` records the
server's identifier and the moment it had, so the push sees a note it has
already sent and that has not been edited since (ADR-0019's `isPushed` /
`isEditedSincePush`). That guard predates this record — its comment already
said "which is the cursor another device pulls on", written before any device
pulled — and it is now pinned by a test, because it is load-bearing and quiet
when it breaks.

### 5. The button does both

Pressing sync means "make the two agree". Push first, so an edit made here
reaches the server before the server's copy is read back.

## A bug this found, which had nothing to do with pulling

Every Turkish character the app read from the server was mangled: `küme testi`
displayed as `kÃ¼me testi`.

`http.Response.body` decodes `bodyBytes` using the `charset` parameter of
`Content-Type` and **falls back to latin-1** when there is none, which the
package documents as RFC 2616 behaviour (http 1.3.0 source, read 2026-10-10).
The server sent `application/json` with no charset. So this was true of every
note, every cluster and every source the app had ever displayed — it was
simply never looked at closely until a pulled note appeared on screen.

Fixed at both ends, deliberately:

- the server declares `application/json; charset=utf-8` on every reply it
  writes;
- the app decodes `bodyBytes` as UTF-8 itself and does not consult the header,
  because only one of the two ends is in this repository's gift once somebody
  runs an older server.

The request direction was already correct: the package defaults a request body
to UTF-8 and adds the charset to the header itself.

## Costs

- **ADR-0012's "one-directional" is now about authorship on this path too.**
  A note is still written on a device and not on the server. Traffic goes both
  ways.
- **The phone is now a replica as well as an author.** Pulling the server's
  whole store onto a phone is a different storage profile from holding only
  what that phone wrote, and nothing bounds it: there is no per-project pull
  and no "only the last N months".
- **Last writer by `updated_at` wins**, with nothing warning anybody, which is
  the same policy ADR-0019 and ADR-0032 settled on. Two devices editing one
  note means one edit is gone.
- **An edit made against a note the server has deleted is lost silently.**
  The push sends it as a `PUT`, the server answers 404, the phone leaves the
  note alone, and the next pull's tombstone removes it. Deletion is the later
  word, which is the policy — but nobody is told that what they just typed
  went nowhere.
- **An unreadable row is skipped silently.** One bad note drops out of a page
  rather than wedging the pull; nothing counts how many.

## A cost this record claimed, checked and withdrawn

The first version of this record said `POST /ingest` has no tombstone guard,
so a device pushing before it pulls could re-create a deleted note under a new
identifier. **That is not true, and the claim is withdrawn.**

A note the server has ever seen carries its identifier, so `_pushNotesBatch`
sends an edit as a `PUT` and never posts it to `/ingest` — the branch that
posts is only reached by a note with no `remote_id`, which by definition the
server has never held and so cannot have deleted. A 404 on the `PUT` is
treated as "there is nothing there", and the note is left for the pull's
tombstone to remove.

Pinned by a test, because a claim in a record that nobody checked is how a
later reader ends up fixing something that was not broken.

## Verified

On an iOS 27 simulator against the Docker server, 2026-10-10, from an empty
phone: ten notes and three tombstones came down on the first sync; `küme
testi` read correctly on screen; a note deleted on the server with
`evomem delete` was gone from the phone after the next sync; the phone's
`deletions` table stayed empty; and the server still held exactly what it had,
with no duplicate from the round trip.
