package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Two machines sharing one mirror, which is the whole point of ADR-0015.
// One deletes a note; the other has to stop having it.
func twoMachines(t *testing.T) (*Worker, *database.DB, *Worker, *database.DB, *fakeRemote) {
	t.Helper()
	remote := newFakeRemote()

	open := func() (*Worker, *database.DB) {
		db, err := database.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		w, err := New(db, remote, Config{BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		return w, db
	}

	a, aDB := open()
	b, bDB := open()
	return a, aDB, b, bDB, remote
}

// The item this started from: a note deleted on one machine is still on the
// other. Before the mirror kept tombstones there was nothing to read, so it
// stayed there for ever.
func TestADeletionReachesTheOtherMachine(t *testing.T) {
	ctx := context.Background()
	a, aDB, b, bDB, _ := twoMachines(t)

	note := &models.Note{
		ProjectID: "evomem", Content: "written on A", SourceType: models.SourceManual,
	}
	if err := aDB.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	// B reads it.
	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bDB.Get(ctx, note.ID); err != nil {
		t.Fatalf("B never got the note: %v", err)
	}

	// A deletes it and pushes the tombstone.
	if err := aDB.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	// B pulls again.
	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bDB.Get(ctx, note.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("the note is still on B after it was deleted on A: %v", err)
	}
}

// And the worse half: B still holds the note and its updated_at is ahead of
// B's own push watermark, so B's next push sends it back up. The mirror is
// the only place that knows about the tombstone, so it is the only place
// that can refuse.
func TestAnOutOfDateMachineCannotResurrectADeletedNote(t *testing.T) {
	ctx := context.Background()
	a, aDB, b, bDB, remote := twoMachines(t)

	note := &models.Note{
		ProjectID: "evomem", Content: "written on A", SourceType: models.SourceManual,
	}
	if err := aDB.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	// A deletes it. B has not pulled since, so B still has it.
	if err := aDB.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bDB.Get(ctx, note.ID); err != nil {
		t.Fatalf("this test needs B to still be holding the note: %v", err)
	}

	// B pushes before it pulls, which is the order RunOnce uses.
	if _, err := b.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	if remote.has(note.ID) {
		t.Error("an out-of-date machine pushed a deleted note back and undid the deletion")
	}
}

// A tombstone applied here must not be recorded here. Recording it would
// push it straight back to the mirror that reported it, on every pass, for
// ever.
func TestAnAppliedTombstoneIsNotPushedBack(t *testing.T) {
	ctx := context.Background()
	a, aDB, b, bDB, _ := twoMachines(t)

	note := &models.Note{
		ProjectID: "evomem", Content: "written on A", SourceType: models.SourceManual,
	}
	if err := aDB.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if err := aDB.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	_, pending, err := bDB.PendingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("B has %d deletion(s) to push after only ever receiving one", pending)
	}
}

// Pulling tombstones has to settle, the way pulling notes does.
func TestPullingDeletionsTwiceIsQuiet(t *testing.T) {
	ctx := context.Background()
	a, aDB, b, _, _ := twoMachines(t)

	note := &models.Note{
		ProjectID: "evomem", Content: "written on A", SourceType: models.SourceManual,
	}
	if err := aDB.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := aDB.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, err := b.Pull(ctx); err != nil {
			t.Fatalf("pull %d: %v", i+1, err)
		}
	}
}

// A machine that never saw the note still gets told about the deletion. That
// is not an error: the mirror tells everyone.
func TestForgettingANoteThatWasNeverHereIsNotAnError(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Forget(ctx, "01M4JGGGGGGGGGGGGGGGGGGGGG"); err != nil {
		t.Errorf("forgetting a note that was never here: %v", err)
	}
}

// The order within one pull: a note written and then deleted on the mirror
// arrives in both batches, and the deletion has to be the later word.
func TestANoteDeletedAfterItWasWrittenDoesNotSurviveOnePull(t *testing.T) {
	ctx := context.Background()
	_, _, b, bDB, remote := twoMachines(t)

	const id = "01M4JHHHHHHHHHHHHHHHHHHHHH"
	at := time.Now().UTC()
	remote.notes[id] = &models.Note{
		ID: id, ProjectID: "evomem", Content: "written then deleted",
		SourceType: models.SourceManual, CreatedAt: at, UpdatedAt: at,
	}
	if err := remote.PushDeletions(ctx, []database.Deletion{
		{ID: id, ProjectID: "evomem", DeletedAt: at.Add(time.Second)},
	}); err != nil {
		t.Fatal(err)
	}
	// PushDeletions took the note off the mirror; put it back, so this
	// one pull sees both the note and the tombstone.
	remote.notes[id] = &models.Note{
		ID: id, ProjectID: "evomem", Content: "written then deleted",
		SourceType: models.SourceManual, CreatedAt: at, UpdatedAt: at,
	}

	if _, err := b.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bDB.Get(ctx, id); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("the note survived a pull that carried its own tombstone: %v", err)
	}
}

// The same two questions, asked of the one transport that ships. The fake
// is my own code and agrees with my own mistakes; PostgreSQL does not.
//
// Needs EVOMEM_TEST_POSTGRES_DSN; skipped without it.
func TestDeletionsTravelThroughPostgres(t *testing.T) {
	ctx := context.Background()
	remote := openRemote(t)

	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w, err := New(db, remote, Config{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}

	const id = "01M4JKKKKKKKKKKKKKKKKKKKKK"
	at := time.Now().UTC()
	note := &models.Note{
		ID: id, ProjectID: "evomem", Content: "on the mirror",
		SourceType: models.SourceManual, CreatedAt: at, UpdatedAt: at,
	}
	if err := remote.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(ctx, id); err != nil {
		t.Fatalf("the note never arrived: %v", err)
	}

	// Somebody else deletes it.
	if err := remote.PushDeletions(ctx, []database.Deletion{
		{ID: id, ProjectID: "evomem", DeletedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(ctx, id); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("the deletion did not arrive: %v", err)
	}

	// Settles rather than repeating.
	for i := 0; i < 3; i++ {
		if _, err := w.Pull(ctx); err != nil {
			t.Fatalf("pull %d after catching up: %v", i+1, err)
		}
	}

	// And the mirror refuses to take the note back from a machine that
	// has not heard about the deletion.
	if err := remote.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}
	var back int
	if err := remote.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM evomem_notes WHERE id = $1`, id).Scan(&back); err != nil {
		t.Fatal(err)
	}
	if back != 0 {
		t.Error("PostgreSQL took a note it holds a tombstone for; the deletion is undone")
	}
}
