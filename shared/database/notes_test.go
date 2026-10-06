package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

func newNote(project, content string) *models.Note {
	return &models.Note{ProjectID: project, Content: content, SourceType: models.SourceManual}
}

func TestCreateFillsInTheBlanks(t *testing.T) {
	db := openTemp(t)
	n := newNote("evomem", "ilk not")

	before := time.Now().Add(-time.Second)
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}

	if !models.ValidULID(n.ID) {
		t.Errorf("identifier %q is not a ULID", n.ID)
	}
	if n.CreatedAt.Before(before) {
		t.Errorf("created_at is %s, which predates the call", n.CreatedAt)
	}
	if !n.UpdatedAt.Equal(n.CreatedAt) {
		t.Errorf("updated_at is %s and created_at is %s; a new note has not been updated",
			n.UpdatedAt, n.CreatedAt)
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "Türkçe içerik: ağ geçidi çöktü")
	n.SourceType = models.SourceTelegram
	n.SetMeta("chat_id", "4242")
	n.SetMeta("voice", true)
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := db.Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != n.Content {
		t.Errorf("content is %q, want %q", got.Content, n.Content)
	}
	if got.SourceType != models.SourceTelegram {
		t.Errorf("source is %q, want telegram", got.SourceType)
	}
	if v, ok := got.MetaString("chat_id"); !ok || v != "4242" {
		t.Errorf("chat_id came back as %q (%v)", v, ok)
	}
	if !got.CreatedAt.Equal(n.CreatedAt) {
		t.Errorf("created_at is %s, want %s", got.CreatedAt, n.CreatedAt)
	}
}

// A caller may supply its own identifier, in either case. Two spellings of
// one identifier have to be one row, because SQLite compares TEXT byte by
// byte and would otherwise allow both.
func TestCreateNormalizesASuppliedID(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	id := models.NewULID()
	n := newNote("evomem", "verilen kimlik")
	n.ID = strings.ToLower(id)
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if n.ID != id {
		t.Errorf("identifier stored as %q, want %q", n.ID, id)
	}
	if _, err := db.Get(ctx, strings.ToLower(id)); err != nil {
		t.Errorf("the lower-case spelling did not find it: %v", err)
	}
}

func TestCreateRejectsDuplicateID(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	id := models.NewULID()
	first := newNote("evomem", "birinci")
	first.ID = id
	if err := db.Create(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := newNote("evomem", "ikinci")
	second.ID = id
	if err := db.Create(ctx, second); err == nil {
		t.Error("the same identifier was accepted twice")
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, nil); err == nil {
		t.Error("a nil note was accepted")
	}
	if err := db.Create(ctx, newNote("", "icerik")); !errors.Is(err, models.ErrEmptyProjectID) {
		t.Errorf("got %v, want ErrEmptyProjectID", err)
	}
	if err := db.Create(ctx, newNote("evomem", "  ")); !errors.Is(err, models.ErrEmptyContent) {
		t.Errorf("got %v, want ErrEmptyContent", err)
	}
}

func TestGetNotFound(t *testing.T) {
	db := openTemp(t)

	if _, err := db.Get(context.Background(), models.NewULID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
	if _, err := db.Get(context.Background(), "nonsense"); !errors.Is(err, models.ErrInvalidULID) {
		t.Errorf("got %v, want ErrInvalidULID", err)
	}
}

func TestUpdate(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "once")
	n.SourceType = models.SourceJira
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	created := n.CreatedAt

	n.Content = "sonra"
	n.SetMeta("status", "done")
	// Fields the caller must not be able to change by passing a different
	// value: the note's identity, not its current text.
	n.ProjectID = "pendra"
	n.SourceType = models.SourceManual
	if err := db.Update(ctx, n); err != nil {
		t.Fatal(err)
	}

	got, err := db.Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "sonra" {
		t.Errorf("content is %q, want sonra", got.Content)
	}
	if v, _ := got.MetaString("status"); v != "done" {
		t.Errorf("status is %q, want done", v)
	}
	if got.ProjectID != "evomem" {
		t.Errorf("project is %q; Update must not move a note", got.ProjectID)
	}
	if got.SourceType != models.SourceJira {
		t.Errorf("source is %q; Update must not change what produced a note", got.SourceType)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("created_at moved to %s", got.CreatedAt)
	}
	if !got.UpdatedAt.After(created) && !got.UpdatedAt.Equal(created) {
		t.Errorf("updated_at is %s, which is before created_at %s", got.UpdatedAt, created)
	}
}

func TestUpdateNotFound(t *testing.T) {
	db := openTemp(t)

	n := newNote("evomem", "yok")
	n.ID = models.NewULID()
	if err := db.Update(context.Background(), n); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "silinecek")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the note is still there: %v", err)
	}
	// A second delete is reported, not swallowed: the caller was working
	// from an identifier that no longer means anything.
	if err := db.Delete(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestCreateBatch(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	notes := []*models.Note{
		newNote("evomem", "biri"),
		newNote("evomem", "ikisi"),
		newNote("pendra", "ucu"),
	}
	if err := db.CreateBatch(ctx, notes); err != nil {
		t.Fatal(err)
	}
	for i, n := range notes {
		if !models.ValidULID(n.ID) {
			t.Errorf("note %d has no identifier", i)
		}
		if _, err := db.Get(ctx, n.ID); err != nil {
			t.Errorf("note %d is not readable: %v", i, err)
		}
	}
}

// One bad note in a batch must take the whole batch with it, or a partially
// ingested webhook payload leaves the store in a state nobody can reason
// about.
func TestCreateBatchIsAllOrNothing(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	good := newNote("evomem", "gecerli")
	notes := []*models.Note{good, newNote("evomem", "")}
	if err := db.CreateBatch(ctx, notes); err == nil {
		t.Fatal("a batch with an invalid note was accepted")
	}

	count, err := db.Count(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d notes were stored, want none", count)
	}
}

func TestCreateBatchEmpty(t *testing.T) {
	db := openTemp(t)
	if err := db.CreateBatch(context.Background(), nil); err != nil {
		t.Errorf("an empty batch was an error: %v", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 5; i++ {
		n := newNote("evomem", fmt.Sprintf("not %d", i))
		if err := db.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	got, err := db.List(ctx, ListOptions{ProjectID: "evomem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d notes, want 5", len(got))
	}
	for i, n := range got {
		want := ids[len(ids)-1-i]
		if n.ID != want {
			t.Errorf("position %d is %s, want %s", i, n.ID, want)
		}
	}
}

func TestListFilters(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	a := newNote("evomem", "evomem notu")
	b := newNote("pendra", "pendra notu")
	c := newNote("evomem", "telegramdan")
	c.SourceType = models.SourceTelegram
	if err := db.CreateBatch(ctx, []*models.Note{a, b, c}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		opts ListOptions
		want int
	}{
		{"everything", ListOptions{}, 3},
		{"one project", ListOptions{ProjectID: "evomem"}, 2},
		{"one source", ListOptions{SourceType: models.SourceTelegram}, 1},
		{"both", ListOptions{ProjectID: "evomem", SourceType: models.SourceManual}, 1},
		{"no such project", ListOptions{ProjectID: "charactly"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.List(ctx, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("got %d notes, want %d", len(got), tc.want)
			}
			count, err := db.Count(ctx, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if count != tc.want {
				t.Errorf("Count gave %d, want %d", count, tc.want)
			}
		})
	}
}

func TestListPaging(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := db.Create(ctx, newNote("evomem", fmt.Sprintf("not %d", i))); err != nil {
			t.Fatal(err)
		}
	}

	first, err := db.List(ctx, ListOptions{Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.List(ctx, ListOptions{Limit: 4, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 4 || len(second) != 4 {
		t.Fatalf("pages are %d and %d long, want 4 and 4", len(first), len(second))
	}
	for _, a := range first {
		for _, b := range second {
			if a.ID == b.ID {
				t.Errorf("%s is on both pages", a.ID)
			}
		}
	}
}

// A caller that asks for nothing gets a bounded answer, and one that asks for
// everything does not get to exhaust memory.
func TestListClampsLimit(t *testing.T) {
	if got := clampLimit(0); got != defaultLimit {
		t.Errorf("clampLimit(0) = %d, want %d", got, defaultLimit)
	}
	if got := clampLimit(-5); got != defaultLimit {
		t.Errorf("clampLimit(-5) = %d, want %d", got, defaultLimit)
	}
	if got := clampLimit(maxLimit * 10); got != maxLimit {
		t.Errorf("clampLimit(huge) = %d, want %d", got, maxLimit)
	}
	if got := clampLimit(7); got != 7 {
		t.Errorf("clampLimit(7) = %d, want 7", got)
	}
}

func TestProjects(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.CreateBatch(ctx, []*models.Note{
		newNote("evomem", "bir"),
		newNote("evomem", "iki"),
		newNote("pendra", "uc"),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d projects, want 2", len(got))
	}
	counts := map[string]int{}
	for _, p := range got {
		counts[p.ProjectID] = p.Notes
		if p.NewestAt.IsZero() {
			t.Errorf("%s has no newest timestamp", p.ProjectID)
		}
	}
	if counts["evomem"] != 2 || counts["pendra"] != 1 {
		t.Errorf("counts are %v, want evomem 2 and pendra 1", counts)
	}
}

// The single writer pool exists so that concurrent ingestion waits instead of
// failing. This is the test that would have caught its removal.
func TestConcurrentWritesDoNotCollide(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	const writers, each = 8, 25
	errs := make(chan error, writers*each)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				n := newNote("evomem", fmt.Sprintf("writer %d note %d", w, i))
				if err := db.Create(ctx, n); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("a concurrent write failed: %v", err)
	}

	count, err := db.Count(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if count != writers*each {
		t.Errorf("%d notes were stored, want %d", count, writers*each)
	}
}

// Reads must not be blocked by a write in flight; that is what WAL and the
// second pool are for.
func TestReadDuringWriteTransaction(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "zaten var")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}

	tx, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET content = 'degisti' WHERE id = ?`, n.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := db.Get(ctx, n.ID)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a read during an open write failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("a read during an open write did not return")
	}
}

func TestTimestampsSortAsText(t *testing.T) {
	// The stored format has to order correctly as a string, because that
	// is how SQLite orders a DATETIME column.
	early := formatTime(time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC))
	late := formatTime(time.Date(2026, 1, 2, 3, 4, 5, 7, time.UTC))
	if !(early < late) {
		t.Errorf("%q does not sort before %q", early, late)
	}

	back, err := parseTime(early)
	if err != nil {
		t.Fatal(err)
	}
	if back.Format(timeLayout) != early {
		t.Errorf("round trip gave %q, want %q", back.Format(timeLayout), early)
	}
}

// A timestamp written by something other than this package still has to be
// readable; the column is DATETIME and SQLite will store whatever it is given.
func TestParseTimeAcceptsPlainRFC3339(t *testing.T) {
	got, err := parseTime("2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2026 {
		t.Errorf("got %s", got)
	}
	if _, err := parseTime("dün"); err == nil {
		t.Error("nonsense was accepted as a timestamp")
	}
}
