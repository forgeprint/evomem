package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/forgeprint/evomem/core/transcribe"
	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// cmdTranscribe turns the recordings in the store into text.
//
// A command rather than a background worker inside serve, so that the thing
// holding a bot token and calling out to two other hosts is something a
// person starts, a timer drives, and anyone can run by hand to see what it
// does. See ADR-0016.
func cmdTranscribe(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	batch := fs.Int("batch", transcribe.DefaultBatch, "how many recordings at most")
	dryRun := fs.Bool("dry-run", false, "say what would be transcribed, change nothing")
	timeout := fs.Duration("timeout", transcribe.DefaultTimeout, "how long one recording may take")
	retry := fs.String("retry", "", "put one note back in the queue and do nothing else")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	if *retry != "" {
		return retryTranscription(ctx, db, *retry, out)
	}

	waiting, err := db.AwaitingTranscriptionCount(ctx)
	if err != nil {
		return err
	}
	if waiting == 0 {
		fmt.Fprintln(out, "nothing is waiting to be transcribed")
		return nil
	}

	svc, err := transcribe.NewService(transcribe.Config{
		BaseURL:  os.Getenv("EVOMEM_TRANSCRIPTION_URL"),
		Token:    os.Getenv("EVOMEM_TRANSCRIPTION_TOKEN"),
		Model:    os.Getenv("EVOMEM_TRANSCRIPTION_MODEL"),
		Language: os.Getenv("EVOMEM_TRANSCRIPTION_LANGUAGE"),
		Timeout:  *timeout,
	})
	if err != nil {
		// Not configured is the feature being off, not a mistake. The
		// count is still worth saying: it is what would happen if it
		// were switched on.
		if errors.Is(err, transcribe.ErrNotConfigured) {
			fmt.Fprintf(out, "%d %s waiting, and transcription is off.\n",
				waiting, plural(waiting, "recording", "recordings"))
			fmt.Fprintln(out, "Set EVOMEM_TRANSCRIPTION_URL to an OpenAI-compatible")
			fmt.Fprintln(out, "transcription service to turn it on.")
			return nil
		}
		return err
	}

	fetchers := map[models.SourceType]transcribe.Fetcher{}

	// Recordings uploaded to /ingest/audio are on this disk, so this one
	// needs no credential and is always available.
	local, err := transcribe.NewLocalFiles(recordings())
	if err != nil {
		return err
	}
	fetchers[models.SourceAudio] = local

	telegram, err := transcribe.NewTelegramFiles(
		os.Getenv("EVOMEM_TELEGRAM_BOT_TOKEN"),
		os.Getenv("EVOMEM_TELEGRAM_API_URL"),
	)
	switch {
	case err == nil:
		fetchers[models.SourceTelegram] = telegram
	case errors.Is(err, transcribe.ErrNotConfigured):
		// Left out of the map on purpose: Run reports each note it
		// cannot reach rather than failing the run, so a store holding
		// both Telegram and some future source still makes progress.
		fmt.Fprintln(out, "EVOMEM_TELEGRAM_BOT_TOKEN is unset, so Telegram recordings cannot be downloaded")
	default:
		return err
	}

	fmt.Fprintf(out, "%d %s waiting; service %s\n\n",
		waiting, plural(waiting, "recording", "recordings"), svc.Endpoint())

	started := time.Now()
	result, err := transcribe.Run(ctx, db, svc, fetchers, transcribe.Options{
		Batch:  *batch,
		DryRun: *dryRun,
	}, out)
	if err != nil {
		return err
	}

	fmt.Fprintf(out,
		"\n%d considered, %d transcribed, %d silent, %d given up on, %d failed, %d skipped in %s\n",
		result.Considered, result.Transcribed, result.Silent, result.Abandoned,
		result.Failed, result.Skipped,
		time.Since(started).Round(time.Millisecond))
	if result.Failed > 0 {
		fmt.Fprintln(out, "\nA failure stays marked as waiting and says why on the note;")
		fmt.Fprintln(out, "the next run tries it again.")
	}
	if result.Silent > 0 || result.Abandoned > 0 {
		fmt.Fprintln(out, "\nSilent and given-up notes have stopped waiting, so no run")
		fmt.Fprintln(out, "picks them up again. `evomem transcribe -retry <id>` puts one back.")
	}
	return nil
}

// retryTranscription puts one note back in the queue.
//
// What makes giving up safe: a service that was misconfigured, or briefly
// answering nothing, would otherwise have abandoned notes with no way back
// (ADR-0039). One identifier at a time and on purpose — a flag that put
// everything back would undo a day of correct answers as easily as a bad
// batch.
func retryTranscription(ctx context.Context, db *database.DB, id string, out io.Writer) error {
	note, err := db.Get(ctx, id)
	if err != nil {
		return err
	}
	if !note.RetryTranscription() {
		fmt.Fprintf(out, "%s was not given up on and is not marked silent; nothing to put back\n", id)
		return nil
	}
	if err := db.Update(ctx, note); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s is waiting to be transcribed again\n", id)
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
