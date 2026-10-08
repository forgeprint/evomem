# ADR-0016: Transcription is an optional HTTP service evomem does not own

**Status:** accepted · **Date:** 2026-10-08 · Supersedes the proposed draft
of the same number.

## Context

Telegram voice and audio messages are stored with a readable description —
`Voice message, 0:14` — plus `telegram_file_id` and
`awaiting_transcription: true`. Nothing transcribes them. They are recorded
and never read.

Transcription needs a speech model, and no speech model can be vendored into
a Go binary. That is the whole difficulty: every option costs something the
project has so far refused to pay.

### What the earlier draft got wrong

The draft chose "self-hosted faster-whisper, with local Whisper.cpp as
fallback" — two integrations, one of which needs cgo and so collides with
ADR-0001. It put the work in `core/api/adapters/transcription` as a
`POST /transcribe` endpoint *evomem serves*, which is backwards: evomem is
the client here, and serving an unauthenticated multipart upload contradicts
ADR-0010. It configured the speech server's model and device through
evomem's own environment, for a service evomem does not own. And it left the
one question that blocks any code unanswered: how a transcript enters memory.

### What the Telegram API allows

From the Bot API reference, checked 2026-10-08 against Bot API 10.3 (24
August 2026), `File` section:

- **The maximum file size to download is 20 MB.** A longer recording cannot
  be fetched at all.
- A `file_path` is **guaranteed valid for at least 1 hour**, and "when the
  link expires, a new one can be requested by calling getFile".
- Downloads come from `https://api.telegram.org/file/bot<token>/<file_path>`,
  so fetching needs the **bot token**, which `evomem serve` does not have
  today — it knows only the webhook secret.

Because the link is short-lived but `file_id` is not, the fetch belongs at
transcription time rather than at webhook time.

## Decision

### 1. One integration: an OpenAI-compatible HTTP service

Evomem posts audio to a transcription service over HTTP and reads back text,
against the interface OpenAI documents and every self-hosted server
implements: `POST /v1/audio/transcriptions`, multipart with a `file` part and
a `model` field, `Authorization: Bearer <token>`, and a reply whose `text`
holds the transcript. Checked 2026-10-08 against
<https://developers.openai.com/api/docs/guides/speech-to-text>.

Which implementation serves that URL — faster-whisper, whisper.cpp behind a
socket, something not yet written — is the operator's business and is not
decided here.

**Deviation from the first draft of this decision.** It said evomem would
know "a URL and an optional token, nothing else. No model name." That did not
survive contact with the interface: `model` is a **required field of the
request**, not server configuration, so there is no version of this that does
not send one. `EVOMEM_TRANSCRIPTION_MODEL` exists, defaulted to `whisper-1`,
the model every compatible server implements. `EVOMEM_TRANSCRIPTION_LANGUAGE`
is there for the same reason and is worth setting: a model left to guess the
language of short Turkish audio returns a confident translation of something
nobody said. What stays refused is configuring the *service's own* internals —
device, beam size, compute type — which remain the operator's.

There is **no local fallback**. A second code path that needs cgo, or a
second binary shipped alongside, costs more than it returns for a feature
that degrades cleanly to nothing.

### 2. It is optional, and its absence is not an error

With no `EVOMEM_TRANSCRIPTION_URL`, nothing changes: voice notes keep their
description and `awaiting_transcription: true`, exactly as today. The binary
on its own stays fully functional with no runtime dependency, which is the
property ADR-0002 and the project's local-first posture exist to protect.
Transcription is the one feature that can be switched off by not configuring
it.

### 3. A new command runs the loop: `evomem transcribe`

Not a background worker inside `evomem serve`. The same shape as
`evomem sync`: it scans for notes with `awaiting_transcription: true`,
fetches each file, posts it, writes the result, and exits. launchd and
systemd drive it on a timer, and `scripts/` already holds that pattern for
sync.

This keeps `evomem serve` what it is — a request handler that writes a row
and returns — rather than a long-lived process holding a bot token, making
outbound calls and competing for the single writer.

### 4. A transcript overwrites the content and says it is a transcript

The note's `content` becomes the transcript, because `content` is what the
FTS index holds and what a model is shown; a transcript nobody can search is
not worth fetching.

Note that a Telegram note is **already tainted**: every adapter calls
`MarkTainted`, so `tainted: true` and `origin: telegram` are already on these
rows and stay there. That mark says a third party wrote the text. It does not
say a machine derived it, which is a different claim and the one that matters
for a lossy transcript. So, in addition:

- `transcribed: true` and `transcribed_at` are set.
- `transcription_source` records the service URL that produced it.
- The description the row was created with is kept in
  `transcription_replaced`, so what arrived is not lost.
- `awaiting_transcription` is removed, which is what keeps the command
  idempotent.
- The MCP tools surface `transcribed` the way they already surface `tainted`,
  in `structuredContent` and as a line in the text, because a model shown a
  machine transcription as if it were typed words will treat a mishearing as
  something the user said.

### 5. Failures are recorded on the note, not retried blindly

A file over 20 MB, a service that is down, audio the model returns nothing
for: `transcription_error` and `transcription_attempted_at` go in the
metadata and `awaiting_transcription` **stays set**. The next run tries
again; an operator can see why from the row. A note is never left in a state
where nobody can tell what happened to it.

## Consequences

### Costs accepted

- **A new secret.** `EVOMEM_TELEGRAM_BOT_TOKEN`, from the environment and
  never a flag, per ADR-0010. `evomem serve` does not need it; only
  `evomem transcribe` does.
- **A new outbound dependency** on `api.telegram.org` at transcription time.
  Until now evomem only ever received from Telegram.
- **Recordings over 20 MB are never transcribed.** Not a bug to fix: the
  limit is the API's. The note says so in `transcription_error`.
- **A bad transcript is searchable as if it were the note.** The marks say it
  is a machine transcription, but a reader who ignores them reads a mishearing
  as the user's words. The alternative — a proposal per voice note — would
  mean a human decision for every dictation, which is most of the reason to
  dictate.
- **Nothing clears the marks.** There is no command today that endorses a
  transcript and removes `tainted`, and none is invented here. A transcript
  stays marked for its lifetime. Whether an endorsement flow should exist is a
  separate decision.
- **One more thing an operator has to run.** A speech service, on their own
  hardware, with its own lifecycle.
- **The whole recording is held in memory.** Up to the 20 MB bound, which is
  therefore evomem's bound as much as the API's. Streaming it through would
  save the memory and lose the ability to refuse a file before posting it.

### Explicitly not decided here

- **The mobile app's own recordings.** Phase 3 saves audio files locally with
  references in the Flutter database. Those files are not on the server and
  there is no path that would carry them there. Reaching them needs an upload
  route that does not exist, and that is its own ADR.
- **Which speech implementation.** Including whether it runs on CPU or a GPU.
  The draft's claim that faster-whisper is "fast enough on CPU" was never
  measured and is not repeated here.
- **The service's own wire format.** Written against the chosen service's
  current documentation when the code is written, cited there, per the rule
  against writing a payload shape from memory.

## Configuration

```
EVOMEM_TRANSCRIPTION_URL       # absent: transcription is off
EVOMEM_TRANSCRIPTION_TOKEN     # optional, if the service wants one
EVOMEM_TRANSCRIPTION_MODEL     # required by the interface; default whisper-1
EVOMEM_TRANSCRIPTION_LANGUAGE  # ISO-639-1 hint, such as tr
EVOMEM_TELEGRAM_BOT_TOKEN      # needed to download what Telegram holds
EVOMEM_TELEGRAM_API_URL        # a self-hosted Bot API server, if not Telegram's
```
