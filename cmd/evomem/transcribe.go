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
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

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

	fmt.Fprintf(out, "\n%d considered, %d transcribed, %d failed, %d skipped in %s\n",
		result.Considered, result.Transcribed, result.Failed, result.Skipped,
		time.Since(started).Round(time.Millisecond))
	if result.Failed > 0 {
		fmt.Fprintln(out, "\nA failure stays marked as waiting and says why on the note;")
		fmt.Fprintln(out, "the next run tries it again.")
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
