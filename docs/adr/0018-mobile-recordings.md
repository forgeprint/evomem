# ADR-0018: A phone's recordings are uploaded to evomem and kept there

**Status:** accepted · **Date:** 2026-10-08

## Context

ADR-0016 gave Telegram recordings a route into text and left the mobile app's
own recordings out, saying they "sit on the phone" with no way to reach the
server. This decides that route.

### Two things that were written down and are not true

`docs/plan.md` has this ticked under Phase 3:

> - [x] Implement background-ready audio recording module saving raw files
>   locally to native storage directories and appending references to the
>   local DB.

**There is no recording in the mobile app.** No recording package in
`pubspec.yaml`, no microphone permission in either platform manifest, no
capture UI, no file handling. The only occurrence of the word "audio" in
`apps/mobile/lib` is a doc comment listing the source types. ADR-0016 then
repeated the claim, saying Phase 3 "saves audio files locally with references
in the Flutter database". Both are corrected here and in the plan.

So this decision is about a path for something that does not exist yet. That
is still worth deciding now: the shape of the path determines what the
recording code has to do, and ADR-0016 already left a seam for it — `Fetcher`
in `core/transcribe` is keyed by source type precisely so a second source
could be added without a second transcription path.

### What the phone can do today

`SyncService` posts notes to `POST /ingest` with a bearer token, one note per
request, JSON. The phone already holds the server URL and the token, so
reaching evomem needs no new credential.

### The problem that blocks any of this

`/ingest` does not take an id. It mints a ULID server-side and returns it:

```json
{"status": "ok", "id": "01M4D3H3HNMFM69N4MHNAYBZ1X", "project": "evomem"}
```

**The phone throws that away.** `_pushNotesBatch` checks for a 201 and reads
nothing else, so every note exists under one id on the phone and a different
one on the server, and the phone does not know the second. Two consequences:

- A recording cannot be attached to anything, because the phone cannot name
  the note the server stored.
- The push is not idempotent. A batch that fails after some notes were
  accepted re-sends them on the next run and the server stores them again,
  under new ids, as new notes.

The second is a pre-existing bug and not caused by audio. It is named here
because audio cannot work until the first is fixed, and the same fix covers
both.

## Decision

### 1. The phone uploads the audio to evomem; `evomem transcribe` does the rest

The alternative was the phone posting straight to the transcription service
and syncing only the text. It buys nothing: the transcription service
normally runs beside the PostgreSQL mirror, behind the same tunnel the phone
already reaches evomem through, so the phone would need a second credential
and a second copy of the transcription logic in Dart to end up in the same
place.

Uploading instead means a Telegram voice message and a phone recording follow
one path from here on: a note marked `awaiting_transcription`, a `Fetcher`
that knows how to reach its audio, one `evomem transcribe`.

### 2. Two steps, because the id comes from the server

```
POST /ingest                      -> {"id": "01M4D..."}   the note
POST /ingest/audio?note=01M4D...  -> 204                   the recording
```

The note is stored first and the recording is attached to the id that came
back. A single multipart request carrying both would avoid the round trip,
but `/ingest` already has two body formats for Apple Shortcuts' sake and a
third would make the endpoint harder to read than this is to send.

Between the two requests the note exists with `awaiting_transcription: true`
and no audio. `evomem transcribe` reports that as a failure and leaves it
queued, which is the right behaviour for an upload that never arrived.

**Prerequisite:** the Flutter side keeps the returned id — in a `remote_id`
column, since the local id is what the phone's own rows are keyed by — and
sends the audio against it. This also makes the push idempotent: a note that
already has a `remote_id` is not posted again.

### 3. The endpoint fails closed, like every other one

`POST /ingest/audio` is registered only when `EVOMEM_API_TOKEN` is set,
alongside `/ingest`, and takes the same bearer token. No token, no endpoint —
a 404 at a known address, never an unauthenticated write (ADR-0010).

**Deviation from ADR-0010, accepted here:** that record bounds this package's
own endpoints at 1 MiB. Audio does not fit. This endpoint is bounded at 20
MiB instead — the same figure as the Telegram download limit, and the same
reason: `core/transcribe` reads a whole recording into memory, so the bound
is evomem's as much as any sender's. It applies to this endpoint only; 1 MiB
stands everywhere else.

A `?note=` that is not a ULID, or names a note that does not exist, is a 400
rather than a file written under an attacker-chosen name.

### 4. Recordings live beside the database

`<directory of EVOMEM_DB>/audio/<note-id>.<ext>`, the extension taken from
the upload's content type. One file per note, named by an id the server
minted, so nothing a sender chooses becomes a path.

A new `Fetcher` reads them, registered for `source_type: audio`. It needs no
network and no token.

### 5. The recording is kept after it is transcribed

Not deleted. A transcript is a lossy derivative, and keeping the original
means a bad one can be listened to and redone with a better model later.

This is the expensive choice, and the costs are below rather than hidden.

## Consequences

### Costs accepted

- **The server's disk grows without bound.** Nothing deletes a recording.
  Voice notes are the largest thing evomem stores by two orders of magnitude,
  and a year of daily dictation is gigabytes next to a database that is
  megabytes.
- **Archiving does not cover audio.** ADR-0012 thins the database of notes
  older than six months; this decision does not extend it to the files. An
  archived note can therefore leave its recording behind with nothing
  pointing at it. Fixing that is a change to ADR-0012 and is not made here.
- **Deleting a note must delete its recording.** Otherwise content a person
  deleted survives on disk, which is worse than a waste of space. The delete
  path writes a tombstone today and knows nothing about files; it has to.
  This is the one consequence that is not optional, because it is about
  deleted data rather than about disk.
- **The mirror does not carry audio.** ADR-0012 mirrors rows. `evomem pull`
  and `evomem restore` bring back the note and its transcript, and the
  recording is not there. A second machine restored from the mirror has the
  words and not the voice.
- **A 20 MiB endpoint.** Larger than anything else evomem accepts, on a
  server that sits behind a tunnel.

### Explicitly not decided here

- **How the phone records.** The package, the audio format, the microphone
  permission flow on each platform, and what the capture UI looks like. Those
  belong to the session that writes the recorder, against the packages'
  current documentation. The plan's false tick is corrected to unticked.
- **Playback.** Nothing plays a recording back, on the phone or anywhere
  else. Keeping the file is what makes that possible later; it is not this.
- **Audio from anywhere else.** `/ingest/audio` is for a note evomem already
  stored. Whether some other automation may upload a recording is a question
  nobody has asked yet.
