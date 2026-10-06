package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

func TestSchemaVersionIsRecorded(t *testing.T) {
	db := openTemp(t)

	got, err := db.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("schema version is %d, want 2", got)
	}
}

// A version bump without a migration slot, or the other way round, is caught
// before it can leave a file half-upgraded.
func TestMigrationStepsMatchTheVersion(t *testing.T) {
	if len(migrations) != schemaVersion-1 {
		t.Errorf("%d migration steps for schema version %d", len(migrations), schemaVersion)
	}
}

// An older file has to come forward, keeping what is in it.
func TestUpgradeFromSchemaOne(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "written under schema 1")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	// Put the file back to how version 1 left it: the tables version 2
	// added, gone, and the version number with them.
	for _, stmt := range []string{
		`DROP TABLE deletions`, `DROP TABLE sync_state`, `DROP INDEX idx_notes_updated`,
		`UPDATE meta SET value = 1 WHERE key = 'schema_version'`,
	} {
		if _, err := db.write.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.migrate(ctx); err != nil {
		t.Fatalf("upgrading from schema 1 failed: %v", err)
	}
	version, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("version is %d after the upgrade, want %d", version, schemaVersion)
	}
	if _, err := db.Get(ctx, n.ID); err != nil {
		t.Errorf("the note did not survive the upgrade: %v", err)
	}
	// The new tables have to be usable, not just present.
	if _, _, err := db.PendingNotes(ctx, Cursor{}, 10); err != nil {
		t.Errorf("the sync tables are not usable after the upgrade: %v", err)
	}
}

// Forward only. An older build must not touch a file a newer one wrote: it
// does not know what the new objects mean, and this file is the only copy of
// the user's notes.
func TestRefusesANewerFile(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if _, err := db.write.ExecContext(ctx,
		`UPDATE meta SET value = ? WHERE key = 'schema_version'`, schemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.migrate(ctx); err == nil {
		t.Error("a file from a newer build was accepted")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	// Never written is the zero cursor, which means everything is
	// pending.
	got, err := db.Cursor(ctx, CursorNotes)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("an unwritten cursor is %v, want the zero value", got)
	}

	want := Cursor{UpdatedAt: time.Now().UTC().Truncate(time.Millisecond), ID: models.NewULID()}
	if err := db.SetCursor(ctx, CursorNotes, want); err != nil {
		t.Fatal(err)
	}
	got, err = db.Cursor(ctx, CursorNotes)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("got %v, want %v", got, want)
	}

	// The two cursors are separate watermarks.
	other, err := db.Cursor(ctx, CursorDeletions)
	if err != nil {
		t.Fatal(err)
	}
	if !other.IsZero() {
		t.Errorf("setting the notes cursor moved the deletions cursor to %v", other)
	}
}

// A cursor that went backwards would re-send everything after it, on every
// pass, forever. Refusing is cheaper than noticing it on a bill.
func TestCursorWillNotGoBackwards(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	now := time.Now().UTC()
	first := Cursor{UpdatedAt: now, ID: "B"}
	if err := db.SetCursor(ctx, CursorNotes, first); err != nil {
		t.Fatal(err)
	}

	for _, back := range []Cursor{
		{UpdatedAt: now.Add(-time.Hour), ID: "Z"},
		{UpdatedAt: now, ID: "A"},
		first,
	} {
		if err := db.SetCursor(ctx, CursorNotes, back); err == nil {
			t.Errorf("the cursor was moved back to %v", back)
		}
	}

	// Forward on either component is fine.
	if err := db.SetCursor(ctx, CursorNotes, Cursor{UpdatedAt: now, ID: "C"}); err != nil {
		t.Errorf("moving the id forward failed: %v", err)
	}
	if err := db.SetCursor(ctx, CursorNotes, Cursor{UpdatedAt: now.Add(time.Second), ID: "A"}); err != nil {
		t.Errorf("moving the timestamp forward failed: %v", err)
	}
}

func TestSetCursorRejectsEmpty(t *testing.T) {
	db := openTemp(t)
	if err := db.SetCursor(context.Background(), CursorNotes, Cursor{}); err == nil {
		t.Error("an empty cursor was accepted")
	}
}

func TestPendingNotes(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 5; i++ {
		n := newNote("evomem", fmt.Sprintf("note %d", i))
		if err := db.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	// Nothing synced: everything is pending, oldest change first, which
	// is the order in which a partial send still leaves a correct
	// watermark.
	notes, next, err := db.PendingNotes(ctx, Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 5 {
		t.Fatalf("got %d pending notes, want 5", len(notes))
	}
	for i, n := range notes {
		if n.ID != ids[i] {
			t.Errorf("position %d is %s, want %s", i, n.ID, ids[i])
		}
	}
	if next.ID != ids[4] {
		t.Errorf("the returned cursor is %s, want the last note %s", next.ID, ids[4])
	}

	// After committing that cursor there is nothing left.
	if err := db.SetCursor(ctx, CursorNotes, next); err != nil {
		t.Fatal(err)
	}
	notes, _, err = db.PendingNotes(ctx, next, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("%d notes are still pending after the cursor moved", len(notes))
	}
}

func TestPendingNotesBatches(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := db.Create(ctx, newNote("evomem", fmt.Sprintf("note %d", i))); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	cursor := Cursor{}
	for pass := 0; pass < 10; pass++ {
		notes, next, err := db.PendingNotes(ctx, cursor, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(notes) == 0 {
			break
		}
		for _, n := range notes {
			if seen[n.ID] {
				t.Errorf("%s was returned twice", n.ID)
			}
			seen[n.ID] = true
		}
		cursor = next
	}
	if len(seen) != 10 {
		t.Errorf("%d notes were walked, want 10", len(seen))
	}
}

// An edited note is pending again: updated_at moved past the cursor.
func TestEditedNoteIsPendingAgain(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "first")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	_, next, err := db.PendingNotes(ctx, Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetCursor(ctx, CursorNotes, next); err != nil {
		t.Fatal(err)
	}

	n.Content = "second"
	if err := db.Update(ctx, n); err != nil {
		t.Fatal(err)
	}

	cursor, err := db.Cursor(ctx, CursorNotes)
	if err != nil {
		t.Fatal(err)
	}
	notes, _, err := db.PendingNotes(ctx, cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("%d notes are pending after an edit, want 1", len(notes))
	}
	if notes[0].Content != "second" {
		t.Errorf("the pending note says %q", notes[0].Content)
	}
}

// Once the row is gone there is nothing left to tell the far end it went, so
// the tombstone is the only record that the note was deleted rather than
// never written.
func TestDeleteLeavesATombstone(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "to be deleted")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}

	deletions, next, err := db.PendingDeletions(ctx, Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletions) != 1 {
		t.Fatalf("%d tombstones, want 1", len(deletions))
	}
	if deletions[0].ID != n.ID {
		t.Errorf("the tombstone is for %s, want %s", deletions[0].ID, n.ID)
	}
	// The project travels with it so the far end can scope the delete
	// without having to know the note.
	if deletions[0].ProjectID != "evomem" {
		t.Errorf("the tombstone's project is %q", deletions[0].ProjectID)
	}
	if deletions[0].DeletedAt.IsZero() {
		t.Error("the tombstone has no timestamp")
	}
	if next.ID != n.ID {
		t.Errorf("the returned cursor is %s", next.ID)
	}
}

// A failed delete must not leave a tombstone for a note that is still there.
func TestFailedDeleteLeavesNoTombstone(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Delete(ctx, models.NewULID()); err == nil {
		t.Fatal("deleting an unknown note succeeded")
	}
	deletions, _, err := db.PendingDeletions(ctx, Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletions) != 0 {
		t.Errorf("%d tombstones were left by a failed delete", len(deletions))
	}
}

func TestPendingCount(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	notes, deletions, err := db.PendingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if notes != 0 || deletions != 0 {
		t.Errorf("an empty store has %d notes and %d deletions pending", notes, deletions)
	}

	for i := 0; i < 3; i++ {
		if err := db.Create(ctx, newNote("evomem", fmt.Sprintf("n%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	gone := newNote("evomem", "gone")
	if err := db.Create(ctx, gone); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	notes, deletions, err = db.PendingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if notes != 3 {
		t.Errorf("%d notes pending, want 3", notes)
	}
	// A store with nothing new and unsent deletes is not idle.
	if deletions != 1 {
		t.Errorf("%d deletions pending, want 1", deletions)
	}
}

// --- archiving ---

// createdAt writes a note with a chosen creation time, which is what
// archiving selects on.
func createdAt(t *testing.T, db *DB, project string, when time.Time) *models.Note {
	t.Helper()
	n := &models.Note{
		ProjectID: project, Content: "note from " + when.Format("2006-01"),
		SourceType: models.SourceManual,
		CreatedAt:  when, UpdatedAt: when,
	}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n
}

// syncEverything moves the cursors past everything, as a completed sync
// would.
func syncEverything(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	if _, next, err := db.PendingNotes(ctx, Cursor{}, maxLimit); err != nil {
		t.Fatal(err)
	} else if !next.IsZero() {
		if err := db.SetCursor(ctx, CursorNotes, next); err != nil {
			t.Fatal(err)
		}
	}
	if _, next, err := db.PendingDeletions(ctx, Cursor{}, maxLimit); err != nil {
		t.Fatal(err)
	} else if !next.IsZero() {
		if err := db.SetCursor(ctx, CursorDeletions, next); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchiveNeedsACutoff(t *testing.T) {
	db := openTemp(t)
	if _, err := db.Archive(context.Background(), ArchiveOptions{}); err == nil {
		t.Error("archiving with no cutoff was accepted")
	}
}

// Archiving is meant to thin a file whose contents exist in the cloud. A note
// past the sync cursor exists only here, and removing it would not be
// archiving but deleting.
func TestArchiveKeepsUnsyncedNotes(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	old := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))

	result, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesRemoved != 0 {
		t.Errorf("%d unsynced notes were removed", result.NotesRemoved)
	}
	// The number that explains a run which seemed to do nothing.
	if result.Skipped != 1 {
		t.Errorf("Skipped is %d, want 1", result.Skipped)
	}
	if _, err := db.Get(ctx, old.ID); err != nil {
		t.Errorf("an unsynced note was lost: %v", err)
	}
}

// And when the caller says, in as many words, that it means to lose the data.
func TestArchiveIncludeUnsynced(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	old := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))

	result, err := db.Archive(ctx, ArchiveOptions{
		Before:          time.Now().AddDate(0, -6, 0),
		IncludeUnsynced: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesRemoved != 1 {
		t.Errorf("%d notes removed, want 1", result.NotesRemoved)
	}
	if _, err := db.Get(ctx, old.ID); err == nil {
		t.Error("the note is still there")
	}
}

func TestArchiveRemovesOnlyWhatIsOld(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	old := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))
	recent := createdAt(t, db, "evomem", time.Now().AddDate(0, -1, 0))
	syncEverything(t, db)

	result, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesRemoved != 1 {
		t.Fatalf("%d notes removed, want 1", result.NotesRemoved)
	}
	if _, err := db.Get(ctx, old.ID); err == nil {
		t.Error("the old note is still there")
	}
	if _, err := db.Get(ctx, recent.ID); err != nil {
		t.Errorf("a recent note was archived: %v", err)
	}
}

func TestArchiveByProject(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	mine := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))
	theirs := createdAt(t, db, "pendra", time.Now().AddDate(0, -8, 0))
	syncEverything(t, db)

	if _, err := db.Archive(ctx, ArchiveOptions{
		Before: time.Now().AddDate(0, -6, 0), ProjectID: "evomem",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(ctx, mine.ID); err == nil {
		t.Error("the targeted note is still there")
	}
	if _, err := db.Get(ctx, theirs.ID); err != nil {
		t.Errorf("another project's note was archived: %v", err)
	}
}

// A tombstone means "the user removed this", and the far end acts on it by
// deleting the cloud copy — the one copy archiving relies on. Archiving must
// not leave one.
func TestArchiveLeavesNoTombstones(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))
	syncEverything(t, db)

	if _, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)}); err != nil {
		t.Fatal(err)
	}

	cursor, err := db.Cursor(ctx, CursorDeletions)
	if err != nil {
		t.Fatal(err)
	}
	deletions, _, err := db.PendingDeletions(ctx, cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletions) != 0 {
		t.Errorf("archiving left %d tombstone(s); the far end would delete the cloud copy", len(deletions))
	}
}

// The deletions table is otherwise the one thing in the store that only ever
// grows.
func TestArchiveRemovesSyncedTombstones(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	gone := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))
	if err := db.Delete(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	// Back-date the tombstone so it is older than the cutoff.
	if _, err := db.write.ExecContext(ctx, `UPDATE deletions SET deleted_at = ?`,
		formatTime(time.Now().AddDate(0, -8, 0))); err != nil {
		t.Fatal(err)
	}
	syncEverything(t, db)

	result, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if result.TombstonesRemoved != 1 {
		t.Errorf("%d tombstones removed, want 1", result.TombstonesRemoved)
	}
}

// An unsynced tombstone must survive: the far end has not been told yet.
func TestArchiveKeepsUnsyncedTombstones(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	gone := createdAt(t, db, "evomem", time.Now().AddDate(0, -8, 0))
	if err := db.Delete(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.write.ExecContext(ctx, `UPDATE deletions SET deleted_at = ?`,
		formatTime(time.Now().AddDate(0, -8, 0))); err != nil {
		t.Fatal(err)
	}

	result, err := db.Archive(ctx, ArchiveOptions{
		Before: time.Now().AddDate(0, -6, 0), IncludeUnsynced: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TombstonesRemoved != 0 {
		t.Errorf("%d unsynced tombstones were removed", result.TombstonesRemoved)
	}
}

// Archiving removes rows, and the full-text index has to shrink with them or
// a search keeps finding notes that are gone.
func TestArchiveShrinksTheSearchIndex(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := &models.Note{
		ProjectID: "evomem", Content: "arsivlenecekkelime", SourceType: models.SourceManual,
		CreatedAt: time.Now().AddDate(0, -8, 0), UpdatedAt: time.Now().AddDate(0, -8, 0),
	}
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	syncEverything(t, db)

	if _, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)}); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "arsivlenecekkelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("an archived note is still searchable: %d hits", len(hits))
	}
}

// VACUUM is where ADR-0005's explicit rowid earns itself: it may renumber an
// implicit rowid, and the index is joined to notes by rowid. If this test
// ever finds the wrong note, that decision was undone.
func TestVacuumKeepsTheIndexAligned(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		n := &models.Note{
			ProjectID: "evomem", Content: fmt.Sprintf("eski kayit %d", i),
			SourceType: models.SourceManual,
			CreatedAt:  time.Now().AddDate(0, -8, 0), UpdatedAt: time.Now().AddDate(0, -8, 0),
		}
		if err := db.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	keep := newNote("evomem", "kalan kayit onyedi")
	if err := db.Create(ctx, keep); err != nil {
		t.Fatal(err)
	}
	syncEverything(t, db)

	result, err := db.Archive(ctx, ArchiveOptions{
		Before: time.Now().AddDate(0, -6, 0), Vacuum: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesRemoved != 20 {
		t.Fatalf("%d notes removed, want 20", result.NotesRemoved)
	}
	if result.BytesBefore == 0 || result.BytesAfter == 0 {
		t.Errorf("a vacuum reported no sizes: %+v", result)
	}

	// The surviving note must still be findable, and the hit must be it.
	hits, err := db.Search(ctx, SearchQuery{Text: "kalan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("%d hits after the vacuum, want 1", len(hits))
	}
	if hits[0].Note.ID != keep.ID {
		t.Errorf("the search found %s, want %s; the index and the table have come apart",
			hits[0].Note.ID, keep.ID)
	}
	if hits[0].Note.Content != keep.Content {
		t.Errorf("the hit's content is %q, want %q", hits[0].Note.Content, keep.Content)
	}
}
