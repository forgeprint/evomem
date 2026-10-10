package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// DefaultBatch is how many notes one run works through. A run is driven by a
// timer, so there is no reason for one to take an hour; what it does not
// reach, the next one does.
const DefaultBatch = 20

// A Fetcher produces the audio behind one note.
//
// One implementation per source. Telegram's is TelegramFiles; the mobile
// app's recordings would need another, and there is no path that carries them
// to the server yet (ADR-0016 leaves that out).
type Fetcher interface {
	// Fetch returns the audio and a filename whose extension says what
	// the bytes are.
	Fetch(ctx context.Context, ref string) ([]byte, string, error)
}

// Options bound one run.
type Options struct {
	// Batch is how many notes to work through. Zero means DefaultBatch.
	Batch int

	// DryRun reports what would be transcribed and changes nothing.
	DryRun bool
}

// Result is what a run did, for the command to print.
type Result struct {
	// Considered is how many notes were waiting and in this batch.
	Considered int

	// Transcribed is how many notes now hold a transcript.
	Transcribed int

	// Failed is how many recorded a reason instead and are still waiting.
	Failed int

	// Skipped is how many could not be attempted at all, because no
	// fetcher knows how to reach their audio.
	Skipped int
}

// Run transcribes what is waiting.
//
// A failure on one note never stops the run: the reason goes on that note and
// the next one is tried. A run that aborted on the first unreachable
// recording would leave the rest of the queue untouched for as long as that
// one note stayed broken.
func Run(
	ctx context.Context,
	db *database.DB,
	svc *Service,
	fetchers map[models.SourceType]Fetcher,
	opts Options,
	log io.Writer,
) (Result, error) {
	var result Result

	batch := opts.Batch
	if batch <= 0 {
		batch = DefaultBatch
	}

	waiting, err := db.AwaitingTranscription(ctx, batch)
	if err != nil {
		return result, err
	}
	result.Considered = len(waiting)

	for _, note := range waiting {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		fetcher, ref, where, ok := reach(note, fetchers)
		if !ok {
			result.Skipped++
			fmt.Fprintf(log, "%s  skipped: nothing here fetches %s\n", note.ID, where)
			continue
		}

		if opts.DryRun {
			fmt.Fprintf(log, "%s  would transcribe %s audio (%s)\n",
				note.ID, note.SourceType, ref)
			continue
		}

		audio, filename, err := fetcher.Fetch(ctx, ref)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			result.Failed++
			if err := fail(ctx, db, note, err.Error(), opts, log); err != nil {
				return result, err
			}
			continue
		}

		text, err := svc.Transcribe(ctx, filename, bytes.NewReader(audio))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			result.Failed++
			if err := fail(ctx, db, note, err.Error(), opts, log); err != nil {
				return result, err
			}
			continue
		}

		note.ApplyTranscript(text, svc.Endpoint(), time.Now())
		if err := db.Update(ctx, note); err != nil {
			return result, err
		}
		result.Transcribed++
		fmt.Fprintf(log, "%s  transcribed\n", note.ID)
	}

	return result, nil
}

// reach picks what can fetch this note's audio, and what to fetch it with.
//
// **By where the audio can be, not by what the note's source is called.**
// That distinction is the whole of this function and it was wrong: the
// fetcher used to be chosen by `source_type`, so only a note filed as
// `audio` could reach a recording on this disk. But `POST /ingest/audio`
// stores a recording under the **note's identifier** whatever the note's
// source is (ADR-0018), and the ordinary way to make one is two requests —
// `/ingest` creates the note, the upload follows. A note created that way is
// `manual` or `shortcut`, so its recording was stored, served by
// `GET /notes/{id}/audio`, playable in the app, and invisible to this.
//
// A Telegram file_id wins when there is one: that recording lives on
// Telegram's servers and was never written here.
//
// Otherwise the local store is asked. It answers ErrNoRecording for a note
// it holds nothing for, which is recorded on the note rather than skipped —
// a note awaiting a transcript with no reachable audio would otherwise be
// retried for ever in silence.
func reach(n *models.Note, fetchers map[models.SourceType]Fetcher) (Fetcher, string, string, bool) {
	if ref, ok := n.MetaString("telegram_file_id"); ok && ref != "" {
		f, ok := fetchers[models.SourceTelegram]
		return f, ref, "telegram recordings", ok
	}
	// A Telegram note with no file_id has no audio anywhere: the recording
	// is on Telegram's servers and the reference is the only way back to
	// it. Nothing on this disk is named after the note either, because
	// nothing wrote it. Left to the local store, which answers
	// ErrNoRecording, so it is recorded on the note rather than skipped.
	f, ok := fetchers[models.SourceAudio]
	return f, n.ID, "recordings on this disk", ok
}

// fail records on the note why there is still no transcript.
func fail(
	ctx context.Context,
	db *database.DB,
	note *models.Note,
	reason string,
	opts Options,
	log io.Writer,
) error {
	fmt.Fprintf(log, "%s  %s\n", note.ID, reason)
	if opts.DryRun {
		return nil
	}
	note.MarkTranscriptionFailed(reason, time.Now())
	if err := db.Update(ctx, note); err != nil {
		return fmt.Errorf("recording why %s was not transcribed: %w", note.ID, err)
	}
	return nil
}
