package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// fakeFiles stands in for shared/audio, so these tests are about the database
// reaching the recordings rather than about the filesystem.
type fakeFiles struct {
	removed    []string
	removedAll bool
	err        error
}

func (f *fakeFiles) Remove(noteID string) error {
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, noteID)
	return nil
}

func (f *fakeFiles) RemoveAll() error {
	if f.err != nil {
		return f.err
	}
	f.removedAll = true
	return nil
}

func withFiles(t *testing.T) (*DB, *fakeFiles) {
	t.Helper()
	db := openTemp(t)
	files := &fakeFiles{}
	db.SetFiles(files)
	return db, files
}

// The condition ADR-0018 attaches to keeping recordings at all: content a
// person deleted must not survive on disk.
func TestDeleteTakesTheRecording(t *testing.T) {
	db, files := withFiles(t)
	ctx := context.Background()

	n := &models.Note{ProjectID: "evomem", Content: "Voice message, 0:14", SourceType: models.SourceAudio}
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}

	if len(files.removed) != 1 || files.removed[0] != n.ID {
		t.Errorf("removed %v, want [%s]", files.removed, n.ID)
	}
}

// A recording that cannot be removed is reported, not swallowed: the row is
// gone and the file is not, which is the state that must never pass
// unnoticed.
func TestDeleteReportsARecordingItCouldNotRemove(t *testing.T) {
	db, files := withFiles(t)
	files.err = errors.New("permission denied")
	ctx := context.Background()

	n := &models.Note{ProjectID: "evomem", Content: "Voice message", SourceType: models.SourceAudio}
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	err := db.Delete(ctx, n.ID)
	if err == nil {
		t.Fatal("a recording that could not be removed was not reported")
	}
	if !strings.Contains(err.Error(), "recording") {
		t.Errorf("err = %v; it should say what was left behind", err)
	}
	// The row still went: the delete itself committed before this.
	if _, err := db.Get(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the note is still there: %v", err)
	}
}

// A note that was not there never reaches the files at all.
func TestDeleteOfANoteThatIsNotThereLeavesFilesAlone(t *testing.T) {
	db, files := withFiles(t)
	err := db.Delete(context.Background(), "01M4D3H3HNMFM69N4MHNAYBZ1Z")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(files.removed) != 0 {
		t.Errorf("removed %v", files.removed)
	}
}

// Every command wires the files up, but a process that did not must still
// work: nothing here may depend on a store being set.
func TestDeleteWithoutFilesConfigured(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	n := &models.Note{ProjectID: "evomem", Content: "text", SourceType: models.SourceManual}
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, n.ID); err != nil {
		t.Errorf("Delete with no file store: %v", err)
	}
}

func TestArchiveTakesTheRecordingsOfWhatItRemoved(t *testing.T) {
	db, files := withFiles(t)
	ctx := context.Background()

	old := &models.Note{
		ProjectID:  "evomem",
		Content:    "Voice message, 0:14",
		SourceType: models.SourceAudio,
		CreatedAt:  time.Now().AddDate(0, -8, 0),
		UpdatedAt:  time.Now().AddDate(0, -8, 0),
	}
	if err := db.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	recent := &models.Note{ProjectID: "evomem", Content: "today", SourceType: models.SourceManual}
	if err := db.Create(ctx, recent); err != nil {
		t.Fatal(err)
	}

	// IncludeUnsynced, because nothing has been synced in this store and
	// archiving would otherwise hold everything back (ADR-0012).
	result, err := db.Archive(ctx, ArchiveOptions{
		Before:          time.Now().AddDate(0, -6, 0),
		IncludeUnsynced: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesRemoved != 1 {
		t.Fatalf("removed %d notes", result.NotesRemoved)
	}
	// Only the note that went, and by name.
	if len(files.removed) != 1 || files.removed[0] != old.ID {
		t.Errorf("removed %v, want [%s]", files.removed, old.ID)
	}
}

func TestArchiveThatRemovedNothingTouchesNoRecording(t *testing.T) {
	db, files := withFiles(t)
	n := &models.Note{ProjectID: "evomem", Content: "today", SourceType: models.SourceManual}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Archive(context.Background(), ArchiveOptions{
		Before:          time.Now().AddDate(0, -6, 0),
		IncludeUnsynced: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(files.removed) != 0 {
		t.Errorf("removed %v", files.removed)
	}
}

// A restore replaces every note, so every recording goes: keeping them would
// leave recordings under identifiers the new rows may reuse.
func TestDeleteAllNotesTakesEveryRecording(t *testing.T) {
	db, files := withFiles(t)
	ctx := context.Background()
	n := &models.Note{ProjectID: "evomem", Content: "Voice message", SourceType: models.SourceAudio}
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteAllNotes(ctx); err != nil {
		t.Fatal(err)
	}
	if !files.removedAll {
		t.Error("the recordings were kept")
	}
}
