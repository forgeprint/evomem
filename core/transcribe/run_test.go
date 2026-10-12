package transcribe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/audio"
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

// agedVoiceNote is a voice note that has been waiting long enough that an
// upload is no longer on its way.
//
// AbandonAfter exists for the window between `/ingest` and the upload that
// follows it, so a test about giving up has to be outside that window or it
// is testing the window instead. Back-dated before Create, because Create
// keeps a created_at it is given and Update does not write that column at
// all — which an earlier version of this helper found out by asserting it.
func agedVoiceNote(t *testing.T, db *database.DB, fileID string) *models.Note {
	t.Helper()
	n := &models.Note{
		ProjectID:  "evomem",
		Content:    "Voice message, 0:14",
		SourceType: models.SourceTelegram,
		CreatedAt:  time.Now().Add(-2 * AbandonAfter),
	}
	n.MarkTainted("telegram")
	n.SetMeta(models.MetaAwaitingTranscription, true)
	if fileID != "" {
		n.SetMeta("telegram_file_id", fileID)
	}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	stored, err := db.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(stored.CreatedAt) <= AbandonAfter {
		t.Fatalf("the store did not keep the back-dated created_at: %s", stored.CreatedAt)
	}
	return stored
}

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
		map[models.SourceType]Fetcher{
			models.SourceTelegram: fakeFetcher{audio: "ogg", name: "v.oga"},
			// The local store is in the map in any real build. The
			// note with no file_id falls through to it and it has
			// nothing, which is what gets written on the note.
			models.SourceAudio: fakeFetcher{err: audio.ErrNoRecording},
		},
		Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 || result.Transcribed != 1 || result.Failed != 1 {
		t.Errorf("result = %+v", result)
	}
}

// A build with no Telegram token cannot reach a Telegram recording. That is
// the build's limitation, not a fault of the note, so it is skipped and left
// unmarked for a build that can.
func TestRunSkipsASourceNothingCanFetch(t *testing.T) {
	db := openStore(t)
	n := voiceNote(t, db, "AwACAgQ")

	result, err := Run(context.Background(), db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{}, Options{}, io.Discard)
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

// The ordinary way a recording is made is two requests: `/ingest` creates the
// note, `/ingest/audio` uploads against the identifier it answered with
// (ADR-0018). A note made that way is `manual` or `shortcut`, never `audio`.
//
// The fetcher used to be chosen by `source_type`, so that recording was
// stored under the note's identifier, served by `GET /notes/{id}/audio`,
// playable in the app — and invisible here. Found by running it: the one note
// on the server that had audio was the one this skipped.
func TestARecordingUploadedAgainstAnyNoteIsTranscribed(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()

	for _, source := range []models.SourceType{
		models.SourceShortcut, models.SourceManual, models.SourceAudio,
	} {
		n := &models.Note{
			ProjectID: "evomem", Content: "Voice note, 0:10", SourceType: source,
		}
		n.SetMeta(models.MetaAwaitingTranscription, true)
		if err := db.Create(ctx, n); err != nil {
			t.Fatal(err)
		}

		result, err := Run(ctx, db, serviceReturning(t, "what was said"),
			map[models.SourceType]Fetcher{
				models.SourceAudio: fakeFetcher{audio: "m4a", name: n.ID + ".m4a"},
			}, Options{}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if result.Transcribed != 1 {
			t.Errorf("source %s: %+v, want one transcribed", source, result)
		}

		stored, err := db.Get(ctx, n.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stored.Content, "what was said") {
			t.Errorf("source %s: content = %q", source, stored.Content)
		}
		if err := db.Delete(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// The failure ADR-0016 chose to retry for ever, which ADR-0039 separates
// out: the recording is not on this disk and nothing is going to write it,
// so asking again asks the same question until somebody notices. Two notes
// on this machine had been failing on every run for days.
func TestARecordingThatIsNowhereIsGivenUpOn(t *testing.T) {
	db := openStore(t)
	note := agedVoiceNote(t, db, "")
	ctx := context.Background()

	result, err := Run(ctx, db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{
			models.SourceAudio: fakeFetcher{
				err: fmt.Errorf("%w: %s", audio.ErrNoRecording, note.ID),
			},
		}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Abandoned != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The queue empties. This is the whole point: a note nothing can
	// transcribe must stop being asked about.
	if stored.AwaitingTranscription() {
		t.Error("a note whose audio is nowhere is still waiting")
	}
	if v, _ := stored.Metadata[models.MetaTranscriptionAbandoned].(bool); !v {
		t.Error("it was not marked as given up on")
	}
	// Why, on the note, so the reason outlives any log.
	if reason, _ := stored.MetaString(models.MetaTranscriptionError); !strings.Contains(reason, "no recording") {
		t.Errorf("reason = %q", reason)
	}
	// The description is still the best thing the note has to say.
	if stored.Content != "Voice message, 0:14" {
		t.Errorf("content = %q", stored.Content)
	}
}

// An ordinary failure is still retried: this separates the permanent case
// out, it does not give up on everything.
func TestAnOrdinaryFailureStillKeepsWaiting(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")
	ctx := context.Background()

	if _, err := Run(ctx, db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{
			models.SourceTelegram: fakeFetcher{err: errors.New("dial tcp: connection refused")},
		}, Options{}, io.Discard); err != nil {
		t.Fatal(err)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.AwaitingTranscription() {
		t.Error("a transient failure stopped the note waiting")
	}
	if _, ok := stored.Metadata[models.MetaTranscriptionAbandoned]; ok {
		t.Error("a transient failure was treated as permanent")
	}
}

// Silence is an answer. ADR-0037 verified that a recording with no speech in
// it comes back empty and that this is correct — whisper's own log said "VAD
// filter removed 00:10.055 of audio". Treating it as a failure left the note
// waiting and spent an inference on the same silence on every run.
func TestARecordingWithNothingSaidInItStopsWaiting(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")
	ctx := context.Background()

	result, err := Run(ctx, db, serviceReturning(t, "   "),
		map[models.SourceType]Fetcher{
			models.SourceTelegram: fakeFetcher{audio: "tone", name: "a.m4a"},
		}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Silent != 1 || result.Failed != 0 || result.Transcribed != 0 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AwaitingTranscription() {
		t.Error("a note with nothing said in it is still waiting")
	}
	if v, _ := stored.Metadata[models.MetaNothingSaid].(bool); !v {
		t.Error("it was not marked as holding no speech")
	}
	// No reason is written, because nothing went wrong.
	if reason, ok := stored.MetaString(models.MetaTranscriptionError); ok {
		t.Errorf("an answer was recorded as an error: %q", reason)
	}
	// Which service said so, so a bad batch can be traced.
	if _, ok := stored.MetaString(models.MetaTranscriptionSource); !ok {
		t.Error("the service that answered was not recorded")
	}
	// An empty content is not a note.
	if stored.Content != "Voice message, 0:14" {
		t.Errorf("content = %q", stored.Content)
	}
}

// What makes giving up safe. Without this, a service that was misconfigured
// for an afternoon would have abandoned every note it touched, permanently
// and silently.
func TestANoteGivenUpOnCanBePutBack(t *testing.T) {
	db := openStore(t)
	note := agedVoiceNote(t, db, "")
	ctx := context.Background()

	if _, err := Run(ctx, db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{
			models.SourceAudio: fakeFetcher{err: audio.ErrNoRecording},
		}, Options{}, io.Discard); err != nil {
		t.Fatal(err)
	}

	stored, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.RetryTranscription() {
		t.Fatal("a note given up on said there was nothing to put back")
	}
	if err := db.Update(ctx, stored); err != nil {
		t.Fatal(err)
	}

	// Waiting again, and the previous verdict is gone rather than sitting
	// beside the new attempt.
	again, err := db.Get(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.AwaitingTranscription() {
		t.Error("it is not waiting again")
	}
	for _, key := range []string{
		models.MetaTranscriptionAbandoned,
		models.MetaNothingSaid,
		models.MetaTranscriptionError,
	} {
		if _, ok := again.Metadata[key]; ok {
			t.Errorf("%s survived the retry", key)
		}
	}

	// And it is transcribed on the next run, which is the proof the retry
	// is worth having.
	result, err := Run(ctx, db, serviceReturning(t, "Bu bir deneme."),
		map[models.SourceType]Fetcher{
			models.SourceAudio: fakeFetcher{audio: "ok", name: "a.m4a"},
		}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcribed != 1 {
		t.Fatalf("result = %+v", result)
	}
}

// A note that was never given up on is not quietly re-queued: the command
// has to be able to say it did nothing.
func TestPuttingBackANoteThatWasNotGivenUpOnDoesNothing(t *testing.T) {
	db := openStore(t)
	note := voiceNote(t, db, "AwACAgQ")

	if note.RetryTranscription() {
		t.Error("a note still waiting claimed to have been put back")
	}
}
