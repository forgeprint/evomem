# ADR-0039: the transcription queue empties, and the language is not pinned

**Status:** accepted · **Date:** 2026-10-12 · **Amends:** ADR-0016, ADR-0037

## Context

Running transcription for real, rather than reading about it, produced three
notes that would never leave the queue:

```text
01M4JFCPS2WGYRMTWN2D7GMJAF  audio: no recording for this note
01M4JFCPSEFFXQXHDCTMX0QKAW  audio: no recording for this note
01M4M2AJRHCQFFV9XSE9ZBA2TN  transcribe: the service returned no text
```

ADR-0016 chose to keep the waiting mark on any failure so a transient one is
retried, and ADR-0037 already recorded that this was unfinished:

> **A failure keeps the waiting mark and retries for ever.** ADR-0016 chose
> that for transient failures, and "the recording does not exist" is not
> transient.

The third line is worse than unfinished. ADR-0037 had already established
that an empty transcript is the **right** answer to a recording with nothing
said in it, and quoted whisper's own log saying so — *"VAD filter removed
00:10.055 of audio"*. So that note was being retried for ever because it had
been answered correctly.

## Decision

### Three endings, not two

`transcribe` now distinguishes a failure from an ending.

- **Failed** — anything that may work next time. The waiting mark stays, the
  reason goes on the note. Unchanged.
- **Given up on** — the recording is not anywhere this machine can reach.
  `transcription_abandoned`, the reason kept, the waiting mark cleared.
- **Nothing said in it** — the service answered and the transcript was
  empty. `transcription_nothing_said`, the service recorded, the waiting
  mark cleared, and **no error written**, because nothing went wrong.

In every case the content is left alone. The description the note was
created with is still the best thing it has to say, and a note whose content
became an empty string would fail validation.

`ErrNoSpeech` exists for this: the empty transcript still has to be an error
at the service boundary, because the caller cannot write an empty content —
but a named one, so a run can tell an answer from a fault.

### Giving up waits a day first, and that is the arbitrary part

`ErrNoRecording` is not reliably permanent, and a test caught this before
the change shipped. The ordinary way to make a recording is two requests:
`/ingest` creates the note, the upload follows (ADR-0018). Between them the
note is waiting and its audio genuinely is nowhere. A run that gave up the
moment it saw `ErrNoRecording` would abandon notes whose audio was seconds
away, and `TestRunKeepsANoteWhoseUploadIsMissing` existed to say so.

So `AbandonAfter = 24h`, measured from the note's `created_at`. It is a
chosen number, not a derived one: far longer than any upload still on its
way, far shorter than the days these two notes spent failing. It is the one
part of this record that is a judgement rather than a measurement.

### `evomem transcribe -retry <id>`, which is what makes giving up safe

Without a way back, a service that was misconfigured for an afternoon would
abandon every note it touched, permanently and silently. The flag clears
both endings and sets the note waiting again, and says so when the note was
not waiting on anything.

One identifier at a time, on purpose. A flag that put everything back would
undo a day of correct answers as easily as a bad batch.

### The language is whisper's to work out

This **reverses** the part of ADR-0016 and ADR-0037 that pinned the language,
at the user's instruction and against the recorded decision — so it was
measured first rather than switched.

ADR-0016's reason was that *"a model left to work out the language of short
Turkish audio returns a confident translation of something nobody said"*.
Measured against `small` on this machine on 2026-10-12, using macOS's Yelda
for Turkish and Albert for English, encoded the way this system encodes
recordings:

| audio | service pinned `tr` | service `auto` |
| - | - | - |
| Turkish speech | correct | **byte-identical, correct** |
| English speech | `"Bu sefer, bu sefer, bu sefer, …"` to the cap | correct English |
| a 1 kHz tone | — | empty |

The feared failure is real and it belongs to **pinning**, not to detection:
a Turkish-pinned service turns English speech into one phrase repeated until
it stops. Detection cost nothing on Turkish — the same bytes out.

ADR-0016's concern is not disproved in general, only for `small` on this
corpus. A smaller model may well behave as it described.

### Two things found while measuring, both recorded because they are not obvious

**The request's `language` parameter does nothing with this service.** The
same audio comes back byte-identical whether evomem sends `language=tr` or
omits it; `hwdsl2/whisper-server` decides from its own `WHISPER_LANGUAGE`.
So `EVOMEM_TRANSCRIPTION_LANGUAGE` is now unset in `compose.yaml` with that
written beside it: setting it there claimed a control this stack does not
have. The variable stays in evomem, because another implementation of the
same API may honour it.

**An empty `WHISPER_LANGUAGE` becomes `auto`.** From the image's own
`run.sh`, line 100, read in the running container on 2026-10-12:

```sh
[ -z "$WHISPER_LANGUAGE" ] && WHISPER_LANGUAGE=auto
```

And `compose.yaml` has to say `${WHISPER_LANGUAGE:-}`, not
`${WHISPER_LANGUAGE:-tr}`: with `:-` an empty value falls back to the
default, so passing an empty variable left it pinned to `tr`. That cost a
restart to notice.

## Costs

- **24 hours is a number somebody chose.** A client that creates a note and
  uploads its audio more than a day later will have the note given up on
  before the audio arrives. Nothing in this system does that today; the
  phone uploads during the same sync.
- **A note given up on needs a person.** The queue empties, which is the
  point, but the note now sits with `transcription_abandoned` and nothing
  surfaces it. `evomem review` does not list these.
- **"Nothing said in it" is trusted.** A service that answers empty because
  it is broken will mark notes silent rather than failing, and the retry
  flag is the only way back. One note at a time, so a bad batch is tedious
  to undo.
- **Detection is per recording, so a bilingual note is still one language.**
  Audio that switches language mid-sentence gets whichever whisper settles
  on, and nothing says which it chose.
- **The language is no longer stated anywhere.** Pinned, `tr` was in the
  compose file where anybody could read it; now the answer depends on the
  audio, and a wrong detection looks like a bad transcript rather than a
  wrong setting.

## Verified

- Three new tests fail with the branches disabled and pass with them
  restored: a recording that is nowhere is given up on, a recording with
  nothing said in it stops waiting, and a note given up on can be put back
  and is then transcribed on the next run.
- An ordinary failure still keeps waiting and is not marked abandoned, so
  this separates the permanent case out rather than giving up on everything.
- `TestRunKeepsANoteWhoseUploadIsMissing` still passes: a note created
  moments ago with no recording stays in the queue.
- The back-dating helper asserts the store kept its `created_at`, which is
  how it was found that `Create` honours a given one and `Update` does not
  write that column at all.
- The language table above was measured end to end against the running
  service, including the restart that proved `${VAR:-tr}` swallows an empty
  value.
