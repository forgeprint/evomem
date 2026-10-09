package connect

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// testKey is the 32 bytes a key has to be, spelled so that it reads as a
// placeholder rather than as somebody's key: a high-entropy literal here
// is indistinguishable from a leaked one, to a scanner and to a reader.
var testKey = strings.Repeat("evomem-test-key-", 2)

func store(t *testing.T) (*database.DB, database.SecretKey) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "evomem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	key, err := database.ParseSecretKey(testKey)
	if err != nil {
		t.Fatal(err)
	}
	return db, key
}

func connect(t *testing.T, db *database.DB, key database.SecretKey, source string) *database.Connection {
	t.Helper()
	conn := &database.Connection{
		SourceType: source,
		ProjectID:  "evomem",
		BaseURL:    "https://example.invalid",
		Account:    "someone@example.com",
		Query:      "project = EVO",
	}
	if err := db.AddConnection(context.Background(), conn, "the-token", key); err != nil {
		t.Fatal(err)
	}
	return conn
}

// fakePuller answers without a network.
type fakePuller struct {
	notes  []*models.Note
	cursor string
	err    error
	secret string
}

func (f *fakePuller) Pull(_ context.Context, _ *database.Connection, secret string) ([]*models.Note, string, error) {
	f.secret = secret
	return f.notes, f.cursor, f.err
}

func note(content string) *models.Note {
	return &models.Note{Content: content, SourceType: models.SourceJira}
}

func TestRunStoresWhatCameBackAndMarksIt(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()
	connect(t, db, key, "jira")

	puller := &fakePuller{notes: []*models.Note{note("EVO-12: sync worker")}, cursor: "page-2"}
	result, err := Run(ctx, db, key, map[string]Puller{"jira": puller}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pulled != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	// The token reached the puller, from the sealed row.
	if puller.secret != "the-token" {
		t.Errorf("the puller was given %q", puller.secret)
	}

	notes, err := db.List(ctx, database.ListOptions{ProjectID: "evomem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("%d notes stored", len(notes))
	}
	// Somebody else's text, arriving without anyone having read it.
	if !notes[0].Tainted() {
		t.Error("a pulled note is not marked as coming from outside")
	}
	if origin, _ := notes[0].MetaString(models.MetaOrigin); origin != "jira" {
		t.Errorf("origin = %q", origin)
	}
	if notes[0].ProjectID != "evomem" {
		t.Errorf("project = %q; the connection decides it", notes[0].ProjectID)
	}

	// And where it got to is saved.
	stored, _ := db.Connections(ctx)
	if stored[0].Cursor != "page-2" || stored[0].LastError != "" {
		t.Errorf("connection = %+v", stored[0])
	}
}

// A failure on one source must not stop the others: leaving every other
// source unsynced because somebody let one token expire is the wrong trade.
func TestRunCarriesOnPastAFailure(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()
	connect(t, db, key, "jira")
	connect(t, db, key, "other")

	result, err := Run(ctx, db, key, map[string]Puller{
		"jira":  &fakePuller{err: errors.New("that token expired")},
		"other": &fakePuller{notes: []*models.Note{note("from the other source")}},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 || result.Failed != 1 || result.Pulled != 1 {
		t.Errorf("result = %+v", result)
	}

	notes, err := db.List(ctx, database.ListOptions{ProjectID: "evomem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Content != "from the other source" {
		t.Errorf("%d notes stored", len(notes))
	}
}

func TestRunRecordsWhyASourceFailed(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()
	conn := connect(t, db, key, "jira")

	puller := &fakePuller{err: errors.New("jira: answered 401 Unauthorized")}
	result, err := Run(ctx, db, key, map[string]Puller{"jira": puller}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Pulled != 0 {
		t.Fatalf("result = %+v", result)
	}

	stored, _ := db.Connections(ctx)
	if !strings.Contains(stored[0].LastError, "401") {
		t.Errorf("last_error = %q", stored[0].LastError)
	}
	// The cursor is left where it was: a failed pull has not moved on.
	if stored[0].Cursor != conn.Cursor {
		t.Errorf("cursor moved to %q on a failure", stored[0].Cursor)
	}
}

func TestRunSaysWhenNothingCanPullThatSource(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()
	connect(t, db, key, "something-else")

	result, err := Run(ctx, db, key, map[string]Puller{"jira": &fakePuller{}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	stored, _ := db.Connections(ctx)
	if !strings.Contains(stored[0].LastError, "pull that source") {
		t.Errorf("last_error = %q", stored[0].LastError)
	}
}

// A key that opens nothing is a configuration problem, not a source that is
// down, and the connection should say which.
func TestRunReportsAKeyThatOpensNothing(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()
	connect(t, db, key, "jira")

	other, err := database.ParseSecretKey(strings.Repeat("another-test-key", 2))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, db, other, map[string]Puller{"jira": &fakePuller{}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	stored, _ := db.Connections(ctx)
	if !strings.Contains(stored[0].LastError, "does not open") {
		t.Errorf("last_error = %q", stored[0].LastError)
	}
}

func TestRunWithNothingConfigured(t *testing.T) {
	db, key := store(t)
	result, err := Run(context.Background(), db, key, map[string]Puller{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 0 || result.Pulled != 0 {
		t.Errorf("result = %+v", result)
	}
}

// A model key lives in the same keyring as the sources (ADR-0027). It has no
// puller and never will; a pull run has to pass over it rather than mark it
// failed, or every run would leave a permanent error on a row that is working
// exactly as intended.
func TestRunSkipsAModelKey(t *testing.T) {
	db, key := store(t)
	ctx := context.Background()

	model := &database.Connection{
		SourceType: database.ModelSourceType,
		ProjectID:  "evomem",
		BaseURL:    "https://model.invalid",
		Query:      "a-model",
	}
	if err := db.AddConnection(ctx, model, "a-model-key", key); err != nil {
		t.Fatal(err)
	}

	result, err := Run(ctx, db, key, map[string]Puller{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 {
		t.Fatalf("the model row was counted as a failed source: %+v", result)
	}
	if result.Considered != 0 {
		t.Fatalf("the model row was counted as a source: %+v", result)
	}

	after, err := db.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].LastError != "" {
		t.Fatalf("a pull run wrote an error onto the model key: %q", after[0].LastError)
	}

	// And a source type that is genuinely unknown is still reported, so
	// the skip above is a named exception rather than a hole.
	unknown := connect(t, db, key, "something-nobody-wrote")
	result, err = Run(ctx, db, key, map[string]Puller{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 {
		t.Fatalf("an unknown source should still fail: %+v", result)
	}
	_ = unknown
}
