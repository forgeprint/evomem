# ADR-0038: a pulled recording is kept on the device

**Status:** accepted · **Date:** 2026-10-11 · **Amends:** ADR-0029, ADR-0034

## Context

The requirement, in the user's words: the app should hold notes **and
recordings** on the device with no internet, and sync with the server's notes
when there is some.

Notes already worked that way. Recordings worked in one direction only.

A recording made on this device is written to the device (ADR-0029) and
uploaded on sync. A recording made on **another** device arrives as a note
with no local audio, and `PlayRecordingButton` fell back to
`GET /notes/{id}/audio` — correctly, which is why ADR-0034 and that endpoint
exist. But the fetched bytes were played and dropped. So a note pulled from
another device needed the network **on every single play**, and offline it
could not be listened to at all.

That is not what "recordings are kept on the device" means, and nothing said
so: the fallback worked, so the gap was invisible until somebody was offline.

## Decision

The fetched bytes are kept on the device, and playback looks in three places
in order: the recording this device made, the copy kept from an earlier
fetch, then the server.

### Keyed by the remote identifier, not written into the note

The obvious implementation — put the new local path in the note's
`local_audio_path` — is wrong, and the reason is in the sync code rather
than anywhere near the player. `SyncService` sends `'metadata': note.metadata`
as it stands, so a handle written there is **pushed to the server** on the
next sync and overwrites the path belonging to the device that actually made
the recording.

So the cache is keyed by the note's remote identifier and lives only in the
recording store. No note is modified, no `updated_at` moves, and nothing
about this reaches the server or another device.

That the local path of a device already travels to the server is a separate
smell, recorded here and not fixed here.

### The media type has to survive, so only known types are kept

The store's `read` returns a hard-coded `recordingMediaType`, which is true
for a file this app's own encoder wrote and false for anything fetched. A
kept copy is therefore named with the extension its type maps to, and read
back by mapping that extension back to a type.

`playbackExtension` falls back to `.m4a` for a type it does not recognise,
so a round trip through it would hand back unknown bytes labelled
`audio/mp4`. On an Apple platform that is worse than not caching: a browser
recording is Opus in WebM, AVFoundation cannot read it, and `playbackRefusal`
— the one thing that explains that in words — only triggers when the type
says WebM. Relabelled, it would stay silent and the platform would fail with
a status code instead.

So `canKeep` exists and a type outside the table is not kept. The bytes in
hand are still played; only the copy is skipped.

The inverse is deliberately lossy: `audio/x-m4a` comes back as `audio/mp4`.
Same container, and the only rule that reads the type asks whether it is
WebM. Preserving the exact spelling would mean a sidecar file for a
difference nobody can hear.

### The identifier is treated as untrusted

It comes from the server, so it never reaches a path as it stands: anything
outside `[A-Za-z0-9_-]`, or longer than 64 characters, is refused rather
than sanitised. A ULID is Crockford base32, so that covers every identifier
this system mints.

### In a browser the cache is in memory

`BlobRecordingStore` keeps it in a map, so a reload loses it. ADR-0029
already says a browser keeps nothing across a reload and this does not
change that; it saves asking the server twice in one session. A cache that
outlived a reload means IndexedDB — a dependency and a storage quota to
answer for — and that is not decided here.

## Costs

- **A second copy of the audio on disk**, for every pulled recording that
  gets played. Nothing prunes it. The recordings directory already grows
  with this device's own recordings and now grows with other devices' too,
  and a device that pulls a large shared memory will feel it.
- **A deleted note's kept copy is not removed.** Deletion reaches the
  recording a device made, by handle (ADR-0018); the kept copy is keyed by
  remote identifier and nothing deletes it. Audio outlives the note that
  stood for it, locally.
- **A recording replaced on the server is not re-fetched.** The kept copy
  wins for ever. Nothing in this system replaces a note's audio today, so
  it costs nothing yet and would be a silent wrong answer the day something
  does.
- **In a browser it buys one session**, not offline playback.
- **Still nothing fetches audio during sync.** A note arrives without its
  recording and the first play needs the network; only the plays after that
  are free. Fetching on sync would mean deciding how much audio a phone
  should download unasked, which is a bigger question than this.

## Verified

Both new behaviours were proven by removing the change and watching the
tests fail with their own reasons — `it was kept, or the next play needs the
network again`, and `the kept copy was not used` — then restoring it.

- A copy kept from the server is read back with the same bytes and the same
  type; a WebM recording is still WebM afterwards, not m4a.
- A type outside the table is not kept, and nothing is written.
- `../../escaped`, `a/b`, `c\d`, an empty identifier and a 65-character one
  are each refused, and the recordings directory stays empty.
- A pulled note's first play makes exactly one request, to
  `/notes/{id}/audio`, with the token in a header and an empty query
  string, and keeps what came back.
- With a kept copy in place, playing it again with a client that fails the
  test if it is called at all still plays the right bytes.
