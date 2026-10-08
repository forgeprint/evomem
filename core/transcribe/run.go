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

		fetcher, ok := fetchers[note.SourceType]
		if !ok {
			result.Skipped++
			fmt.Fprintf(log, "%s  skipped: nothing here fetches %s audio\n",
				note.ID, note.SourceType)
			continue
		}

		ref, ok := audioRef(note)
		if !ok {
			// Nothing to fetch with. Recorded on the note rather
			// than skipped: a note marked as awaiting a transcript
			// with no way to reach the audio is a bug in whatever
			// wrote it, and it would otherwise be retried forever
			// in silence.
			result.Failed++
			if err := fail(ctx, db, note, "no audio reference in metadata", opts, log); err != nil {
				return result, err
			}
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

// audioRef finds what to fetch the audio with.
//
// What the reference is depends on the source. A Telegram recording is named
// by the file_id the webhook recorded; one uploaded to /ingest/audio is named
// by the note itself, because that is what it was stored under (ADR-0018). A
// new source adds its case here and a Fetcher beside it; the metadata column
// is why that needs no migration.
func audioRef(n *models.Note) (string, bool) {
	if n.SourceType == models.SourceAudio {
		return n.ID, true
	}
	if ref, ok := n.MetaString("telegram_file_id"); ok && ref != "" {
		return ref, true
	}
	return "", false
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
