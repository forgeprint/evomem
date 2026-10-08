package transcribe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

func openStore(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "evomem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// voiceNote is what the Telegram adapter writes for a voice message: content
// that describes the recording, the file_id to fetch it with, the awaiting
// mark, and the taint every adapter sets.
func voiceNote(t *testing.T, db *database.DB, fileID string) *models.Note {
	t.Helper()
	n := &models.Note{
		ProjectID:  "evomem",
		Content:    "Voice message, 0:14",
		SourceType: models.SourceTelegram,
	}
	n.MarkTainted("telegram")
	n.SetMeta(models.MetaAwaitingTranscription, true)
	if fileID != "" {
		n.SetMeta("telegram_file_id", fileID)
	}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n
}

type fakeFetcher struct {
	audio string
	name  string
	err   error
}

func (f fakeFetcher) Fetch(context.Context, string) ([]byte, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return []byte(f.audio), f.name, nil
}

func serviceReturning(t *testing.T, text string) *Service {
	t.Helper()
	srv, _ := fakeService(t, http.StatusOK, `{"text":`+quote(text)+`}`)
	svc, err := NewService(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func TestRunWritesTheTranscriptAndClearsTheMark(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")
	ctx := context.Background()

	result, err := Run(ctx, db, serviceReturning(t, "tünel önce ayakta olmalı"),
		map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"}},
		Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcribed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Content != "tünel önce ayakta olmalı" {
		t.Errorf("content = %q", stored.Content)
	}
	if !stored.Transcribed() {
		t.Error("the note does not say it is a transcription")
	}
	// Cleared, which is what stops the next run picking it up again.
	if stored.AwaitingTranscription() {
		t.Error("the note is still marked as awaiting a transcript")
	}
	// What arrived is not lost.
	if replaced, _ := stored.MetaString(models.MetaTranscriptionReplaced); replaced != "Voice message, 0:14" {
		t.Errorf("replaced = %q", replaced)
	}
	// Traceable to what produced it.
	if src, _ := stored.MetaString(models.MetaTranscriptionSource); !strings.HasSuffix(src, "/v1/audio/transcriptions") {
		t.Errorf("source = %q", src)
	}
	// Where the audio came from did not change.
	if !stored.Tainted() {
		t.Error("the taint was dropped")
	}
}

func TestRunIsIdempotent(t *testing.T) {
	db := openStore(t)
	voiceNote(t, db, "AwACAgQ")
	ctx := context.Background()
	fetchers := map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"}}

	if _, err := Run(ctx, db, serviceReturning(t, "once"), fetchers, Options{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	// A second run has nothing to do; without the mark being cleared it
	// would transcribe and bill for the same recording every time the
	// timer fired.
	second, err := Run(ctx, db, serviceReturning(t, "twice"), fetchers, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if second.Considered != 0 {
		t.Errorf("the second run considered %d notes", second.Considered)
	}
}

func TestRunRecordsAFailureAndKeepsWaiting(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")
	ctx := context.Background()

	result, err := Run(ctx, db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{
			models.SourceTelegram: fakeFetcher{err: errors.New("getFile refused: file is gone")},
		}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Transcribed != 0 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Still waiting, so the next run tries again.
	if !stored.AwaitingTranscription() {
		t.Error("a failed note stopped waiting")
	}
	if reason, _ := stored.MetaString(models.MetaTranscriptionError); !strings.Contains(reason, "file is gone") {
		t.Errorf("reason = %q", reason)
	}
	if _, ok := stored.MetaString(models.MetaTranscriptionAttemptedAt); !ok {
		t.Error("no attempt time was recorded")
	}
	// The content was not touched by a failure.
	if stored.Content != "Voice message, 0:14" {
		t.Errorf("content = %q", stored.Content)
	}
}

// One unreachable recording must not stop the queue.
func TestRunCarriesOnAfterAFailure(t *testing.T) {
	db := openStore(t)
	voiceNote(t, db, "")        // nothing to fetch with
	voiceNote(t, db, "AwACAgQ") // fine
	ctx := context.Background()

	result, err := Run(ctx, db, serviceReturning(t, "second one"),
		map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"}},
		Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 || result.Transcribed != 1 || result.Failed != 1 {
		t.Errorf("result = %+v", result)
	}
}

func TestRunSkipsASourceNothingCanFetch(t *testing.T) {
	db := openStore(t)
	n := &models.Note{ProjectID: "evomem", Content: "Recording, 0:30", SourceType: models.SourceAudio}
	n.SetMeta(models.MetaAwaitingTranscription, true)
	n.SetMeta("telegram_file_id", "irrelevant")
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{}}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// Skipped, not failed: nothing is wrong with the note, this build just
	// has no way to reach its audio.
	if result.Skipped != 1 || result.Failed != 0 {
		t.Errorf("result = %+v", result)
	}
	stored, err := db.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.MetaString(models.MetaTranscriptionError); ok {
		t.Error("a skipped note was marked as failed")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")

	var log strings.Builder
	result, err := Run(context.Background(), db, serviceReturning(t, "would be this"),
		map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"}},
		Options{DryRun: true}, &log)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcribed != 0 {
		t.Errorf("result = %+v", result)
	}
	if !strings.Contains(log.String(), "would transcribe") {
		t.Errorf("log = %q", log.String())
	}

	stored, err := db.Get(context.Background(), note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Content != "Voice message, 0:14" || stored.Transcribed() {
		t.Error("a dry run changed the note")
	}
}

func TestRunWorksOldestFirst(t *testing.T) {
	db := openStore(t)
	first := voiceNote(t, db, "one")
	voiceNote(t, db, "two")

	var log strings.Builder
	if _, err := Run(context.Background(), db, serviceReturning(t, "x"),
		map[models.SourceType]Fetcher{models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"}},
		Options{Batch: 1}, &log); err != nil {
		t.Fatal(err)
	}
	// A queue that worked newest first would leave the oldest recording
	// losing to whatever just arrived, run after run.
	if !strings.Contains(log.String(), first.ID) {
		t.Errorf("the oldest note was not the one done: %q", log.String())
	}
}
