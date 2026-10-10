package sync

import (
	"context"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// A pull must not move the watermark that says how far the push has got.
//
// They are two directions through the same (updated_at, id) ordering, but
// over two different tables: one counts local notes that have reached the
// mirror, the other counts mirror notes that have reached here. Sharing a
// row meant a pull set the push watermark to a position in somebody else's
// ordering — and every local note behind it stopped being pending. The note
// is still on this machine, it never reaches the mirror, and nothing
// anywhere says the two disagree.
func TestPullDoesNotSkipNotesThatHaveNotBeenPushed(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 10})
	ctx := context.Background()

	// One note written here and never pushed.
	local := &models.Note{
		ProjectID: "evomem", Content: "written here, never pushed",
		SourceType: models.SourceManual,
	}
	if err := db.Create(ctx, local); err != nil {
		t.Fatal(err)
	}

	// And one note that is already on the mirror, newer than it. Another
	// device wrote this; this machine has never seen it.
	remote.notes["01M4JZZZZZZZZZZZZZZZZZZZZZ"] = &models.Note{
		ID: "01M4JZZZZZZZZZZZZZZZZZZZZZ", ProjectID: "evomem",
		Content: "written on another device", SourceType: models.SourceManual,
		CreatedAt: time.Now().UTC().Add(time.Hour),
		UpdatedAt: time.Now().UTC().Add(time.Hour),
	}

	if _, err := w.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	// Now push. The local note is still pending: nothing has sent it.
	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed == 0 {
		t.Error("the pull moved the push watermark past a note that had never been pushed")
	}
	if !remote.has(local.ID) {
		t.Errorf("note %s is on this machine and will never reach the mirror", local.ID)
	}
}

// And the other way: a push must not move the watermark that says how far
// the pull has got, or the notes the mirror already held are skipped.
func TestPushDoesNotSkipNotesThatHaveNotBeenPulled(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 10})
	ctx := context.Background()

	// Something on the mirror, from before this machine existed.
	remote.notes["01M4JAAAAAAAAAAAAAAAAAAAAA"] = &models.Note{
		ID: "01M4JAAAAAAAAAAAAAAAAAAAAA", ProjectID: "evomem",
		Content: "written on another device", SourceType: models.SourceManual,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
		UpdatedAt: time.Now().UTC().Add(-time.Hour),
	}

	// A note written here now, which pushes and moves the push watermark
	// to a time after the remote note's.
	if err := db.Create(ctx, &models.Note{
		ProjectID: "evomem", Content: "written here", SourceType: models.SourceManual,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Get(ctx, "01M4JAAAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Errorf("the push moved the pull watermark past a note the mirror already held: %v", err)
	}
}

// Restore says it resets the cursor and then pulls everything. If the
// watermark is left at zero, the next pull walks the whole mirror again and
// re-creates every note it just restored.
func TestRestoreLeavesTheWatermarkWhereItPulledTo(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 10})
	ctx := context.Background()

	for _, id := range []string{"01M4JBBBBBBBBBBBBBBBBBBBBB", "01M4JCCCCCCCCCCCCCCCCCCCCC"} {
		remote.notes[id] = &models.Note{
			ID: id, ProjectID: "evomem", Content: "from the mirror",
			SourceType: models.SourceManual,
			CreatedAt:  time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
	}

	if err := w.Restore(ctx); err != nil {
		t.Fatal(err)
	}

	cursor, err := db.Cursor(ctx, database.CursorPulled)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ID == "" {
		t.Error("restore pulled everything and left the watermark at zero, so the next pull does it all again")
	}
}

// A pull must not land an older copy from the mirror on top of an edit made
// here that has not been pushed yet. Local is the authority for what this
// machine writes (ADR-0012); the mirror's ordering says nothing about a
// local edit, so without a check the edit is overwritten — and then pushed
// back over itself.
func TestPullDoesNotOverwriteANewerLocalEdit(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 10})
	ctx := context.Background()

	const id = "01M4JDDDDDDDDDDDDDDDDDDDDD"
	old := time.Now().UTC().Add(-time.Hour)
	remote.notes[id] = &models.Note{
		ID: id, ProjectID: "evomem", Content: "what the mirror still has",
		SourceType: models.SourceManual, CreatedAt: old, UpdatedAt: old,
	}

	// The same note here, edited since.
	if err := db.Create(ctx, &models.Note{
		ID: id, ProjectID: "evomem", Content: "edited here, not pushed yet",
		SourceType: models.SourceManual,
		CreatedAt:  old, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := db.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "edited here, not pushed yet" {
		t.Errorf("the pull overwrote a newer local edit: content is %q", got.Content)
	}
}

// ADR-0015 says pull is safe to run again. It was not: the second pass hit
// a note this machine had pushed itself and Create failed on the identifier.
func TestPullRunsTwiceWithoutFailing(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 10})
	ctx := context.Background()

	at := time.Now().UTC()
	remote.notes["01M4JEEEEEEEEEEEEEEEEEEEEE"] = &models.Note{
		ID: "01M4JEEEEEEEEEEEEEEEEEEEEE", ProjectID: "evomem",
		Content: "from the mirror", SourceType: models.SourceManual,
		CreatedAt: at, UpdatedAt: at,
	}

	if _, err := w.Pull(ctx); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	// A second pull with nothing new must not fail on an empty cursor
	// time, and a third with the same note must not fail on its id.
	if _, err := w.Pull(ctx); err != nil {
		t.Fatalf("second pull: %v", err)
	}

	if err := db.ResetCursor(ctx, database.CursorPulled); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Pull(ctx); err != nil {
		t.Fatalf("pull over a note that is already here: %v", err)
	}
}

// The same question as TestPullRunsTwiceWithoutFailing, asked of the one
// transport that ships. The fake could not have caught the shape of this:
// the remote hands the cursor back in RFC3339Nano and Pull used to send it
// in RFC3339, so `updated_at > $1` re-selected every note written in that
// same whole second. A caught-up pull then tried to set the watermark to
// where it already was, which SetCursor refuses — and the pull failed from
// then on, for ever, with nothing wrong anywhere.
//
// Needs EVOMEM_TEST_POSTGRES_DSN; skipped without it.
func TestPullAgainstPostgresSettlesInsteadOfLooping(t *testing.T) {
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

	// Three notes inside one second, which is the case the truncation
	// made invisible.
	at := time.Now().UTC().Truncate(time.Second).Add(100 * time.Millisecond)
	var notes []*models.Note
	for _, id := range []string{
		"01M4JF1111111111111111111A",
		"01M4JF1111111111111111111B",
		"01M4JF1111111111111111111C",
	} {
		at = at.Add(time.Millisecond)
		notes = append(notes, &models.Note{
			ID: id, ProjectID: "evomem", Content: "same second",
			SourceType: models.SourceManual, CreatedAt: at, UpdatedAt: at,
		})
	}
	if err := remote.PushNotes(ctx, notes); err != nil {
		t.Fatal(err)
	}

	first, err := w.Pull(ctx)
	if err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if first.IsZero() {
		t.Fatal("the first pull read nothing")
	}

	// Caught up. This must be quiet, not an error, and must not move.
	second, err := w.Pull(ctx)
	if err != nil {
		t.Fatalf("a pull that had caught up failed: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("the watermark moved on a pull with nothing new: %s then %s", first.ID, second.ID)
	}

	for _, n := range notes {
		if _, err := db.Get(ctx, n.ID); err != nil {
			t.Errorf("note %s never arrived: %v", n.ID, err)
		}
	}
}
