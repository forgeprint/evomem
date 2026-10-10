# ADR-0037: speech to text on this machine

**Status:** accepted · **Date:** 2026-10-11 · **Amends:** ADR-0016

## Context

Transcription has been written and switched off since ADR-0016: no
`EVOMEM_TRANSCRIPTION_URL`, so voice notes kept their description and their
`awaiting_transcription` mark and nothing happened.

The user asked for it to run **here, with no token**. ADR-0016 already allows
exactly that and it is worth quoting, because "OpenAI-compatible" reads like a
vendor and is not one:

> Which implementation serves that URL — faster-whisper, whisper.cpp behind a
> socket, something not yet written — is the operator's business and is not
> decided here.

> **Token** for the Authorization header, when the service wants one. A
> self-hosted server usually does not. — `core/transcribe/service.go`

So "OpenAI-compatible" is the **wire format**: a multipart `POST` to
`/v1/audio/transcriptions` with `file` and `model`. And "there is no local
fallback" in ADR-0016 means no second code path *inside evomem* — not that a
server cannot run beside it.

**No evomem code had to change for this.** The token was already optional and
the header is only sent when it is set.

## Decision

### A compose service, not a second thing inside the server image

`hwdsl2/whisper-server` (faster-whisper behind the OpenAI-compatible API),
`linux/arm64`, in `compose.yaml`.

The alternative was asked for and measured before being declined: putting it
inside the server image takes that image from **186 MB to roughly 900 MB**,
and the image is Alpine, so the Python wheels faster-whisper needs — built for
glibc — would mean either moving everything to Debian or compiling
whisper.cpp in the image. ADR-0016 refused that in as many words:

> a second code path that needs cgo, or a second binary shipped alongside,
> costs more than it returns

Today's ffmpeg (ADR-0036) is not a precedent against this: it is 608 KiB and
one invocation, not a thousand times that and a Python runtime.

Against that, a second service costs the operator nothing they were not
already doing — one block in the compose file they already run.

### No token, and no port

`WHISPER_API_KEY: ""` turns the image's generated key off, and the service
publishes **no port**. Those two go together: the key is off because nothing
outside the compose network can reach it, and it must stay unpublished
because the key is off. Changing one means changing the other.

### `small`, and the language set rather than guessed

`WHISPER_MODEL: small` (~465 MB). The image defaults to `base`, which is
poor at Turkish. `medium` is better again at a speed that tells on a CPU.

`EVOMEM_TRANSCRIPTION_LANGUAGE: tr`, for the reason ADR-0016 gives: a model
left to work out the language of short Turkish audio returns a confident
translation of something nobody said. **The cost is that an English voice
note will be read as Turkish**, and it is one variable to change.

### A recording is reached by where it is, not by what the note is called

This was a defect, and running it is what found it.

`POST /ingest/audio` stores a recording under the **note's identifier**
whatever the note's source is (ADR-0018), and the ordinary way to make one is
two requests: `/ingest` creates the note, the upload follows. A note made
that way is `manual` or `shortcut`.

The transcription run chose its fetcher by `source_type`, so only a note
filed as `audio` could reach a recording on this disk. The one note on the
server that actually had audio was skipped with *"nothing here fetches
shortcut audio"*, while two notes whose audio no longer existed were queued
to be transcribed.

`reach` now asks where the audio can be: a `telegram_file_id` wins when there
is one, because that recording lives on Telegram's servers and was never
written here; otherwise the local store is asked and answers
`ErrNoRecording` when it holds nothing.

## Costs

- **Another image to pull and keep.** 723 MB on disk plus a 465 MB model in
  a volume, against 186 MB for the server. It is the largest thing in this
  project by a wide margin.
- **It is CPU inference.** 1.5 seconds for a six-second clip on this machine.
  A long recording is minutes, and `evomem transcribe` is a command somebody
  runs, so nothing warns them.
- **`tr` is hard-coded in compose.** A bilingual memory gets one of its
  languages transcribed badly until somebody changes it.
- **A failure keeps the waiting mark and retries for ever.** ADR-0016 chose
  that for transient failures, and "the recording does not exist" is not
  transient: two notes here will fail on every run until somebody clears
  them. Unchanged by this record and still open.
- **The model is fetched from HuggingFace on first start**, so the first run
  of `docker compose up` needs the network even though nothing afterwards
  does. `WHISPER_LOCAL_ONLY` exists for an air-gapped setup and is not set.

## Verified

A sentence spoken by macOS's Turkish voice, encoded to m4a, uploaded against
a note created through `/ingest` — the two-request flow that used to be
invisible to transcription:

```text
said:        Veritabanı kilitlendi, düğüm çözülemedi. Bu bir sesli not denemesidir.
transcribed: veri tabanı kilitlendi, düğüm çözülemedi. Bu bir sesli not denemesidir.
```

1.5 seconds for six seconds of audio. Every Turkish letter survived; the only
difference is a spelling variant of one compound word. The note kept what it
used to say in `transcription_replaced` and recorded which service answered.

Before that, a recording with no speech in it — a 1 kHz tone — came back with
no text, and whisper's own log said why: *"VAD filter removed 00:10.055 of
audio"*. That is the right answer to a file with nothing said in it.
