package transcribe

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/audio"
	"github.com/forgeprint/evomem/shared/models"
)

func audioStore(t *testing.T) *audio.Store {
	t.Helper()
	return audio.StoreBeside(filepath.Join(t.TempDir(), "evomem.db"))
}

func TestLocalFilesNeedsAStore(t *testing.T) {
	if _, err := NewLocalFiles(nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestLocalFilesReadsWhatWasUploaded(t *testing.T) {
	store := audioStore(t)
	const id = "01M4D3H3HNMFM69N4MHNAYBZ1X"
	if _, err := store.Save(id, "audio/m4a", strings.NewReader("m4a-pretend")); err != nil {
		t.Fatal(err)
	}

	local, err := NewLocalFiles(store)
	if err != nil {
		t.Fatal(err)
	}
	recording, name, err := local.Fetch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if string(recording) != "m4a-pretend" {
		t.Errorf("recording = %q", recording)
	}
	// The extension travels, because the service decodes by it.
	if name != id+".m4a" {
		t.Errorf("name = %q", name)
	}
}

func TestLocalFilesSaysWhenTheUploadNeverArrived(t *testing.T) {
	local, err := NewLocalFiles(audioStore(t))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = local.Fetch(context.Background(), "01M4D3H3HNMFM69N4MHNAYBZ1X")
	if !errors.Is(err, audio.ErrNoRecording) {
		t.Errorf("err = %v, want ErrNoRecording", err)
	}
}

// An uploaded recording and a Telegram voice message go through one
// transcription path; this is the whole point of the Fetcher seam.
func TestRunTranscribesAnUploadedRecording(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()

	n := &models.Note{
		ProjectID:  "evomem",
		Content:    "Recording, 0:30",
		SourceType: models.SourceAudio,
	}
	n.SetMeta(models.MetaAwaitingTranscription, true)
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	store := audioStore(t)
	// Named by the note: that is the reference, with no metadata key of
	// its own (ADR-0018).
	if _, err := store.Save(n.ID, "audio/ogg", strings.NewReader("OggS")); err != nil {
		t.Fatal(err)
	}
	local, err := NewLocalFiles(store)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(ctx, db, serviceReturning(t, "dictated into the phone"),
		map[models.SourceType]Fetcher{models.SourceAudio: local}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transcribed != 1 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Content != "dictated into the phone" {
		t.Errorf("content = %q", stored.Content)
	}
	// Not tainted: this is the user's own recording, not a third party's
	// text. The machine-transcription mark is the only claim here.
	if stored.Tainted() {
		t.Error("an uploaded recording was marked tainted")
	}
	if !stored.Transcribed() {
		t.Error("the note does not say it is a transcription")
	}
}

// An upload that never arrived leaves the note queued with a reason, which is
// the right behaviour for the window between the two requests.
func TestRunKeepsANoteWhoseUploadIsMissing(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()

	n := &models.Note{ProjectID: "evomem", Content: "Recording, 0:30", SourceType: models.SourceAudio}
	n.SetMeta(models.MetaAwaitingTranscription, true)
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	local, err := NewLocalFiles(audioStore(t))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, db, serviceReturning(t, "unused"),
		map[models.SourceType]Fetcher{models.SourceAudio: local}, Options{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}

	stored, err := db.Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.AwaitingTranscription() {
		t.Error("the note stopped waiting although no recording arrived")
	}
	if reason, _ := stored.MetaString(models.MetaTranscriptionError); !strings.Contains(reason, "no recording") {
		t.Errorf("reason = %q", reason)
	}
}
