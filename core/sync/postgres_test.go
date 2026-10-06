package sync

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Postgres has to satisfy Remote at compile time, so a change to the
// interface cannot leave the one shipped transport behind.
var _ Remote = (*Postgres)(nil)

func TestOpenPostgresNeedsADSN(t *testing.T) {
	for _, dsn := range []string{"", "   "} {
		if _, err := OpenPostgres(context.Background(), dsn); err == nil {
			t.Errorf("an empty connection string (%q) was accepted", dsn)
		}
	}
}

// The remote schema is a mirror, deliberately not a copy of the local one.
func TestRemoteSchemaShape(t *testing.T) {
	// Safe to run again, because EnsureSchema is.
	for _, stmt := range strings.Split(remoteSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if !strings.Contains(stmt, "IF NOT EXISTS") {
			t.Errorf("not idempotent: %s", stmt)
		}
	}
	// There is nobody for a remote tombstone to inform: the sync is
	// one-directional and the local file is the authority.
	if strings.Contains(remoteSchema, "deletions") {
		t.Error("the mirror has a deletions table")
	}
}

// The guard is what stops an out-of-order delivery putting an older version
// of a note over a newer one.
func TestUpsertGuardsAgainstGoingBackwards(t *testing.T) {
	if !strings.Contains(upsertNote, "WHERE evomem_notes.updated_at <= EXCLUDED.updated_at") {
		t.Error("the upsert has no guard on updated_at")
	}
	if !strings.Contains(upsertNote, "ON CONFLICT (id) DO UPDATE") {
		t.Error("the upsert is not an upsert; a retried batch would duplicate")
	}
}

// postgresDSN is set for the integration test below. Without it these are
// skipped: the rest of this package is tested against a fake remote, and
// nobody should need a database to run go test.
func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("EVOMEM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EVOMEM_TEST_POSTGRES_DSN is not set")
	}
	return dsn
}

// openRemote connects and gives the test an empty mirror.
func openRemote(t *testing.T) *Postgres {
	t.Helper()
	ctx := context.Background()

	pg, err := OpenPostgres(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pg.Close() })

	if err := pg.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.DB().ExecContext(ctx, `TRUNCATE evomem_notes`); err != nil {
		t.Fatal(err)
	}
	return pg
}

// Ping has to say the schema is missing rather than let every push fail, so
// the user gets an error that says what to do.
func TestPostgresPingWithoutSchema(t *testing.T) {
	ctx := context.Background()
	pg, err := OpenPostgres(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()

	if _, err := pg.DB().ExecContext(ctx, `DROP TABLE IF EXISTS evomem_notes`); err != nil {
		t.Fatal(err)
	}
	err = pg.Ping(ctx)
	if err == nil {
		t.Fatal("Ping succeeded with no table")
	}
	if !strings.Contains(err.Error(), "init-remote") {
		t.Errorf("the error does not say what to do: %v", err)
	}

	if err := pg.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pg.Ping(ctx); err != nil {
		t.Errorf("Ping failed after EnsureSchema: %v", err)
	}
}

func TestPostgresEnsureSchemaIsRepeatable(t *testing.T) {
	pg := openRemote(t)
	for i := 0; i < 3; i++ {
		if err := pg.EnsureSchema(context.Background()); err != nil {
			t.Fatalf("EnsureSchema failed on pass %d: %v", i, err)
		}
	}
}

func TestPostgresPushAndRead(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	note := &models.Note{
		ID: models.NewULID(), ProjectID: "evomem",
		Content: "Türkçe içerik: ağ geçidi çöktü", SourceType: models.SourceTelegram,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	note.MarkTainted("telegram")

	if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}

	var (
		project, content, source, metadata string
		created, updated                   time.Time
	)
	err := pg.DB().QueryRowContext(ctx,
		`SELECT project_id, content, source_type, metadata::text, created_at, updated_at
		 FROM evomem_notes WHERE id = $1`, note.ID).
		Scan(&project, &content, &source, &metadata, &created, &updated)
	if err != nil {
		t.Fatal(err)
	}

	if content != note.Content {
		t.Errorf("content is %q, want %q", content, note.Content)
	}
	if project != "evomem" || source != "telegram" {
		t.Errorf("project %q source %q", project, source)
	}
	if !created.UTC().Equal(note.CreatedAt) {
		t.Errorf("created_at is %s, want %s", created.UTC(), note.CreatedAt)
	}
	// The mark has to survive the trip, or the far end cannot tell
	// third-party text from the user's own either.
	if !strings.Contains(metadata, `"tainted": true`) && !strings.Contains(metadata, `"tainted":true`) {
		t.Errorf("the tainted mark did not arrive: %s", metadata)
	}
}

// A batch is retried whole when a push fails halfway, so the same note
// arriving twice must not become two rows.
func TestPostgresPushIsIdempotent(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	note := &models.Note{
		ID: models.NewULID(), ProjectID: "evomem", Content: "once",
		SourceType: models.SourceManual,
		CreatedAt:  time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	for i := 0; i < 3; i++ {
		if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
			t.Fatal(err)
		}
	}

	count, err := pg.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("the remote holds %d rows after three pushes, want 1", count)
	}
}

// An edit arrives as a newer updated_at and replaces the content.
func TestPostgresUpsertAppliesANewerVersion(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	now := time.Now().UTC()
	note := &models.Note{
		ID: models.NewULID(), ProjectID: "evomem", Content: "first",
		SourceType: models.SourceManual, CreatedAt: now, UpdatedAt: now,
	}
	if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}

	note.Content = "second"
	note.UpdatedAt = now.Add(time.Minute)
	if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}

	var content string
	if err := pg.DB().QueryRowContext(ctx,
		`SELECT content FROM evomem_notes WHERE id = $1`, note.ID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if content != "second" {
		t.Errorf("content is %q, want second", content)
	}
}

// And an older one does not. This is the guard on the upsert: an out-of-order
// delivery must not undo an edit.
func TestPostgresUpsertRejectsAnOlderVersion(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	now := time.Now().UTC()
	note := &models.Note{
		ID: models.NewULID(), ProjectID: "evomem", Content: "newer",
		SourceType: models.SourceManual, CreatedAt: now, UpdatedAt: now,
	}
	if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}

	stale := *note
	stale.Content = "older"
	stale.UpdatedAt = now.Add(-time.Hour)
	if err := pg.PushNotes(ctx, []*models.Note{&stale}); err != nil {
		t.Fatal(err)
	}

	var content string
	if err := pg.DB().QueryRowContext(ctx,
		`SELECT content FROM evomem_notes WHERE id = $1`, note.ID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if content != "newer" {
		t.Errorf("content is %q; an older version overwrote a newer one", content)
	}
}

func TestPostgresPushDeletions(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	now := time.Now().UTC()
	note := &models.Note{
		ID: models.NewULID(), ProjectID: "evomem", Content: "to go",
		SourceType: models.SourceManual, CreatedAt: now, UpdatedAt: now,
	}
	if err := pg.PushNotes(ctx, []*models.Note{note}); err != nil {
		t.Fatal(err)
	}

	if err := pg.PushDeletions(ctx, []database.Deletion{
		{ID: note.ID, ProjectID: "evomem", DeletedAt: now},
	}); err != nil {
		t.Fatal(err)
	}

	count, err := pg.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("the remote holds %d rows after the delete", count)
	}
}

// The local side may report a note the remote never saw, which happens
// whenever a note is created and deleted between two passes.
func TestPostgresDeletingWhatIsNotThere(t *testing.T) {
	pg := openRemote(t)

	err := pg.PushDeletions(context.Background(), []database.Deletion{
		{ID: models.NewULID(), ProjectID: "evomem", DeletedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Errorf("deleting an unknown id was an error: %v", err)
	}
}

func TestPostgresEmptyBatches(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	if err := pg.PushNotes(ctx, nil); err != nil {
		t.Errorf("an empty note batch was an error: %v", err)
	}
	if err := pg.PushDeletions(ctx, nil); err != nil {
		t.Errorf("an empty deletion batch was an error: %v", err)
	}
}

// The whole engine over the real transport: a local store, a worker, and a
// remote that ends up holding what the local one does.
func TestPostgresWorkerEndToEnd(t *testing.T) {
	pg := openRemote(t)
	ctx := context.Background()

	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	w, err := New(db, pg, Config{BatchSize: 3})
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for i := 0; i < 7; i++ {
		n := &models.Note{
			ProjectID: "evomem", Content: "kayit " + models.NewULID(),
			SourceType: models.SourceManual,
		}
		if err := db.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 7 {
		t.Errorf("%d notes pushed, want 7", stats.NotesPushed)
	}
	if count, _ := pg.Count(ctx); count != 7 {
		t.Errorf("the remote holds %d rows, want 7", count)
	}

	// A second pass has nothing to do.
	stats, err = w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Empty() {
		t.Errorf("the second pass pushed %+v", stats)
	}

	// A delete travels, and only that note goes.
	if err := db.Delete(ctx, ids[3]); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if count, _ := pg.Count(ctx); count != 6 {
		t.Errorf("the remote holds %d rows after one delete, want 6", count)
	}

	// And an archive that thins the local file leaves the remote alone:
	// that is the copy archiving relies on.
	if _, err := db.Archive(ctx, database.ArchiveOptions{Before: time.Now().Add(time.Hour)}); err != nil {
		// A cutoff in the future is refused by the CLI, not here; the
		// point is that archiving writes no tombstones.
		t.Log(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if count, _ := pg.Count(ctx); count != 6 {
		t.Errorf("the remote holds %d rows after archiving, want 6; archiving must not propagate", count)
	}
}
