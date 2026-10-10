# ADR-0029: a recording in a browser is a blob, not a file

**Status:** accepted · **Date:** 2026-10-09

## Context

The direction is that the whole note lifecycle has to be testable in a
browser before the phone is built out. Recording is the last part of it that
is not: the app compiles for the web and the record button is there, but
pressing it fails.

Two reasons, and only one of them is the obvious one.

`path_provider` has no web implementation, so there is no directory to
record into. That one is known — it is in `docs/durum.md`.

The other is in the package. Read from record_web 2.1.3's own source on
2026-10-09: on the web `start(config, path:)` **ignores the path**, and
`stop()` returns `web.URL.createObjectURL(blob)` — a `blob:` URL, not a file.
So every `File(path)` in the recording controller and in the upload is
reaching for something that was never written.

A third thing follows from the same source and would have been found later
and worse: the recorded format is whatever the browser's `MediaRecorder`
agreed to. The package asks for AAC and lists `audio/mp4;codecs=mp4a` first,
but a browser that does not support it falls back, and the common answer is
`audio/webm;codecs=opus`. The upload has been declaring `audio/m4a` for
everything.

## Decision

### 1. Storage goes behind a port, like the notes did

`RecordingStore` has three calls: where to record, the bytes of a finished
recording, and forget it. `FileRecordingStore` is `path_provider` and
`dart:io`; `BlobRecordingStore` is the browser. A conditional import picks
one, so nothing above this line imports `dart:io` and the web build does not
carry a path it cannot use.

This is the same shape `NotesStore` took in ADR-0022 and for the same
reason: the thing that differs between platforms is where bytes live, and
that is one interface rather than a `kIsWeb` branch in four places.

### 2. The media type travels with the bytes

`read` returns both. On a phone it is `audio/m4a` because that is what the
encoder wrote; in a browser it is whatever the blob says it is. The upload
sends that, instead of the constant it was sending.

The server already accepts it: `shared/audio` maps `audio/webm` to `.webm`,
cuts the `;codecs=` parameter off first, and the OpenAI speech-to-text
interface documents webm as supported (ADR-0016). So nothing on the server
changes — but it only worked by accident until the client stopped lying
about the type.

### 3. A browser recording does not survive a reload

This is the cost and it is not hidden.

A `blob:` URL lives as long as the document that made it. Reload the page
before syncing and the audio is gone, while the note that describes it
remains, marked `awaiting_transcription` for a transcription that can never
happen.

Three things were considered:

- **Keep the bytes in IndexedDB.** It would survive, and it is the right
  answer if browser recording ever stops being a test surface. It is also a
  second store to write, migrate and keep in step with the deletions, for a
  surface whose stated purpose is trying the lifecycle out.
- **Upload the moment the recording stops.** The upload needs the server's
  identifier for the note, which only exists after a sync (ADR-0018), so
  this would mean syncing one note out of band.
- **Refuse to record in a browser.** Which is where this started.

So: the bytes are read into memory as soon as the recording stops, so the
blob URL's own lifetime stops mattering within the session, and the limit is
written here, in `docs/durum.md`, and said in the app when it bites.

**When it bites, it says so.** An upload that finds no bytes marks the note:
the `awaiting_transcription` flag comes off and `recording_lost` goes on.
A note waiting forever for audio nobody has is worse than a note that says
the audio is gone.

## Consequences

### What was checked in a real browser

Not inferred — run in the page the container serves, on 2026-10-09:

- A `blob:` URL answers **200** to both `fetch` and `XMLHttpRequest`, returns
  the exact bytes, and carries the blob's own type in `content-type`. That is
  what makes `BlobRecordingStore` possible with nothing but `package:http`,
  which uses XHR.
- `MediaRecorder.isTypeSupported` in this browser: `audio/mp4;codecs=mp4a`
  **false**, `audio/mp4;codecs=mp4a.40.2` **true**, `audio/webm;codecs=opus`
  **true**, `audio/aac` **false**. So record_web's first choice for AAC fails
  and its second succeeds here; another browser will answer differently, and
  the server maps both `audio/mp4` and `audio/webm` already. That is the
  point of carrying the type rather than naming it.

**Not checked: an actual recording.** That needs a microphone and a person to
grant permission, which no automated run should be doing. The chain above the
microphone is covered by tests against a fake recorder; the microphone itself
is still unproven in a browser, exactly as it is on a phone.

### Costs accepted

- **Audio recorded in a browser and not synced before a reload is lost.**
  Stated above, surfaced on the note, and the reason the phone is still the
  place recordings are made.
- **Bytes sit in memory** between stopping and syncing. One recording at a
  time, bounded by the same 20 MiB the upload is (ADR-0018).
- **The blob URL is not revoked.** Revoking needs `package:web`, which is a
  dependency for one call; the URL is freed when the page goes, which is
  also when the bytes are lost anyway.
- **Two implementations of the store**, tested separately — the file one
  against a temp directory, the browser one not at all in unit tests, since
  no test here has a browser. It is checked by hand, in one.

### Playing it back

A recording you cannot hear is a recording you cannot check, so a note that
stands for one offers a play button.

`RecordingPlayer` is a port like the store, and for the same reason: a
browser already has an audio element and needs nothing added, while a phone
needs a package this app does not have — and cannot currently be built at
all, since there are no `android/` or `ios/` build files. So the browser
implementation is real and the phone one reports that it cannot play, which
is what hides the button. A button that would do nothing is worse than no
button; the package earns its place when the phone can run.

The button asks this device first and the server second. Once a recording
has been uploaded the server is the only copy that survives a reload, so
`GET /notes/{id}/audio` hands it back — behind the same token as everything
else, with the media type worked out from how it was stored rather than
guessed, and `Cache-Control: no-store`, because it is somebody's own audio.

The bytes are fetched and played from a blob this app builds, rather than
pointing an audio element at the URL. An audio element sends no headers, so
a URL would mean the API token in a query string, and that is the wrong
place for it (ADR-0010).

### Explicitly not decided here

- **IndexedDB for recordings**, which is what would make this durable.
- **Transcription in the browser.** Nothing on any client transcribes
  (ADR-0016) and that is unchanged.
- **Keeping browser recordings across a reload.** IndexedDB would do it.
  Syncing covers the same ground with a store that already exists, so this
  stays deferred.

## Amendment, 2026-10-10: the phone fails the same way, and never answers

Running the app on a phone for the first time (ADR-0031) found the same
failure this record was written about, reached from the other side.

In a browser, `record_web` catches a failed start, prints it, and returns
normally — so the caller is told recording began. This record's answer was
to ask `isRecording()` afterwards rather than believe the return.

On iOS, with an audio input CoreAudio refuses, `record`'s `start` does
neither: it **never returns and never throws**. `isRecording()` is never
reached, nothing is logged, and the button simply stops working. Measured on
an iOS 27 simulator on 2026-10-10.

So the recorder calls that talk to audio hardware are bounded — ten seconds,
`recorderPatience` — and a timeout is reported as a failure like any other.
Asking for permission is deliberately left unbounded, because that call is
waiting for a person to read a dialog.

The logbook this record introduced is what made the difference visible both
times: in a browser it showed "started" followed by nothing captured; on a
phone it showed nothing at all, which is what pointed at a call that never
came back.
