# ADR-0036: every recording plays everywhere

**Status:** accepted · **Date:** 2026-10-11 · **Amends:** ADR-0029, ADR-0031

## Context

A recording made in a browser could not be played on a phone.

The browser records **Opus in a WebM container** — ADR-0029 chose it after
measuring that asking a browser for AAC produces a recorder which reports
success and captures nothing. AVFoundation has **no WebM demuxer**; Safari
plays WebM through WebKit's own, which native code cannot reach. So ADR-0031
added a refusal with a sentence naming where it does work, and left the fix
open:

> Transcoding — on upload, or on demand — would fix it and is not decided
> here.

The user decided: **ffmpeg, and every recording playable everywhere.**

The constraint this runs into is in CLAUDE.md, and it is real: *zero runtime
dependencies for the core Go binary*. ADR-0001 and ADR-0002 exist to keep
`evomem` one static file that runs anywhere.

## Decision

### Convert on arrival, into AAC in an m4a

`Store.Save` converts after it has stored the recording, never instead of
storing it. AAC in MP4 is the one format all three clients play: the phone
records it natively, the browser plays it, and `shared/audio` already mapped
it.

The settings match what the phone records — mono, 16 kHz, 32 kbit/s — for the
reason ADR-0029 gives: a transcription model resamples to 16 kHz mono before
it does anything else, so anything more is discarded on arrival and paid for
in bytes on the way.

### Only what Apple platforms cannot read

`.webm` and `.ogg`. **Not** "everything that is not m4a": an mp3 or a wav
plays everywhere already, and re-encoding one would lose quality to no
purpose.

### The binary keeps no runtime dependency; the image carries one

This is the part that matters for CLAUDE.md, and it is not a loophole.

`evomem` looks for ffmpeg and **keeps the recording exactly as it arrived
when there is none**. A local install with no ffmpeg behaves as it did
before: the browser's recording is stored as WebM, the browser plays it, and
the phone says it cannot. Nothing new fails.

The server's Docker image installs ffmpeg — 608 KiB on alpine:3.22 — because
that image is where recordings arrive from a browser. One line in a
Dockerfile is a different thing from a binary that will not start without a
tool on the PATH.

### Losing the audio is worse than one client not playing it

A conversion that fails leaves the recording as it arrived and the upload
still succeeds. The caller is told through `OnConvert`, which the server
writes to its log. The phone's refusal (ADR-0031) stays as the safety net for
exactly this case, and for stores that were never converted.

The original is removed **only after** the new file exists: until then it is
the only copy there is.

### `evomem audio` for what is already stored

Uploads from now on are converted as they land. Recordings that arrived
before, or on a machine with no ffmpeg, are converted by `evomem audio
-convert`. Without the flag it lists what it would do, because rewriting
somebody's recordings is not a thing to do by accident.

## Costs

- **A second lossy pass.** Opus → AAC, both lossy. Measured on a real
  browser recording: peak amplitude 4348 before and 4331 after, 10.06s to
  10.05s. For speech at these settings it is not audible; for anything else
  it would be the wrong trade.
- **The original is gone.** Keeping both would double the storage for every
  browser recording and leave two files per note for `Open` to choose
  between. The converted one is the one everything can read.
- **The Docker image has a runtime dependency it did not have.** It is
  pinned to whatever alpine:3.22 ships and is not vendored, so ADR-0002's
  "the image contains exactly the dependencies reviewable in this repository"
  is now one short of true. Said plainly rather than quietly.
- **ffmpeg is a large attack surface for a converter.** It parses untrusted
  media — a recording arrives from a browser through the API. It runs as the
  unprivileged `evomem` user in the container with no network namespace of
  its own, which is the mitigation available here and not a strong one.
- **Conversion happens in the request.** A long recording holds the upload
  open: 98ms for ten seconds of speech, so a ninety-minute recording would be
  minutes. Bounded at five minutes by `DefaultConvertTimeout`, after which
  the recording is kept unconverted. Doing it after the response would need a
  queue, which this does not have.

## Verified

Against the running server, 2026-10-11, with a real Opus/WebM recording built
from a real phone recording:

- `POST /ingest/audio` with `Content-Type: audio/webm` → the file on disk is
  `.m4a`, converted in 98ms, and the server logged `converted .webm to .m4a`.
- `GET /notes/{id}/audio` answers `Content-Type: audio/mp4`, mono 16 kHz AAC,
  10.05s, peak amplitude 4331 — the same audio.
- `evomem audio` listed a pre-existing `.webm`, and `-convert` replaced it.

And the thing this was all for, on an iOS 27 simulator: the phone pulled that
note and **played it to the end** — playback started at 02:34:47.738 and
reached `StopAtEnd` at 02:34:57.805, 10.07 seconds, the length of the clip.
The same recording in its original WebM was refused by the same app an hour
earlier with the sentence ADR-0031 added. That sentence is still there, for
a store that was never converted.
