package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// fakeRemote is a Remote that records what it was given and can be made to
// fail or go offline.
type fakeRemote struct {
	mu sync.Mutex

	notes     map[string]*models.Note
	deleted   map[string]bool
	pushes    int
	pingCalls int

	offline  bool
	pushErr  error
	failOnce bool
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{notes: map[string]*models.Note{}, deleted: map[string]bool{}}
}

func (f *fakeRemote) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pingCalls++
	if f.offline {
		return errors.New("no route to host")
	}
	return nil
}

func (f *fakeRemote) PushNotes(_ context.Context, notes []*models.Note) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.nextErr(); err != nil {
		return err
	}
	f.pushes++
	for _, n := range notes {
		// An upsert, which is what the interface requires: a retried
		// batch must not duplicate anything.
		f.notes[n.ID] = n
	}
	return nil
}

func (f *fakeRemote) PushDeletions(_ context.Context, deletions []database.Deletion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.nextErr(); err != nil {
		return err
	}
	f.pushes++
	for _, d := range deletions {
		delete(f.notes, d.ID)
		f.deleted[d.ID] = true
	}
	return nil
}

func (f *fakeRemote) nextErr() error {
	if f.failOnce {
		f.failOnce = false
		return errors.New("connection reset")
	}
	return f.pushErr
}

func (f *fakeRemote) Close() error { return nil }

func (f *fakeRemote) PullNotes(_ context.Context, cursorUpdatedAt, cursorID string, limit int) ([]*models.Note, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var filtered []*models.Note
	for _, n := range f.notes {
		if n.UpdatedAt.After(parseTime(cursorUpdatedAt)) || (n.UpdatedAt.Equal(parseTime(cursorUpdatedAt)) && n.ID > cursorID) {
			filtered = append(filtered, n)
		}
	}
	// Sort by updated_at, id
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].UpdatedAt.Equal(filtered[j].UpdatedAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt)
	})

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	var nextUpdatedAt, nextID string
	if len(filtered) > 0 {
		last := filtered[len(filtered)-1]
		nextUpdatedAt = last.UpdatedAt.Format(time.RFC3339Nano)
		nextID = last.ID
	}

	return filtered, nextUpdatedAt, nextID, nil
}

func (f *fakeRemote) PullAll(_ context.Context, limit int, offset int) ([]*models.Note, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var all []*models.Note
	for _, n := range f.notes {
		all = append(all, n)
	}
	// Sort by updated_at, id
	sort.Slice(all, func(i, j int) bool {
		if all[i].UpdatedAt.Equal(all[j].UpdatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].UpdatedAt.Before(all[j].UpdatedAt)
	})

	if offset >= len(all) {
		return []*models.Note{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (f *fakeRemote) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.notes)
}

func (f *fakeRemote) has(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.notes[id]
	return ok
}

func newWorker(t *testing.T, cfg Config) (*Worker, *database.DB, *fakeRemote) {
	t.Helper()
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	remote := newFakeRemote()
	w, err := New(db, remote, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w, db, remote
}

func addNotes(t *testing.T, db *database.DB, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		note := &models.Note{
			ProjectID: "evomem", Content: fmt.Sprintf("note %d", i),
			SourceType: models.SourceManual,
		}
		if err := db.Create(context.Background(), note); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, note.ID)
	}
	return ids
}

func TestNewValidates(t *testing.T) {
	db, _ := database.OpenMemory()
	defer db.Close()

	if _, err := New(nil, newFakeRemote(), Config{}); err == nil {
		t.Error("a worker was built with no store")
	}
	if _, err := New(db, nil, Config{}); err == nil {
		t.Error("a worker was built with no remote")
	}

	w, err := New(db, newFakeRemote(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if w.cfg.Interval != DefaultInterval || w.cfg.BatchSize != DefaultBatchSize {
		t.Errorf("defaults were not applied: %+v", w.cfg)
	}
}

func TestRunOncePushesEverything(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	ids := addNotes(t, db, 5)

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 5 {
		t.Errorf("%d notes pushed, want 5", stats.NotesPushed)
	}
	if remote.count() != 5 {
		t.Errorf("the remote holds %d notes, want 5", remote.count())
	}
	for _, id := range ids {
		if !remote.has(id) {
			t.Errorf("%s did not arrive", id)
		}
	}

	// A second pass has nothing to do: the cursor moved.
	stats, err = w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Empty() {
		t.Errorf("the second pass pushed %+v", stats)
	}
}

func TestRunOnceBatches(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 3})
	addNotes(t, db, 10)

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 10 {
		t.Errorf("%d notes pushed, want 10", stats.NotesPushed)
	}
	if remote.count() != 10 {
		t.Errorf("the remote holds %d notes, want 10", remote.count())
	}
	// Ten notes in batches of three: four pushes, not one.
	if stats.Batches != 4 {
		t.Errorf("%d batches, want 4", stats.Batches)
	}
}

// A laptop in a tunnel is the normal case for a local-first store, so being
// unreachable is a state, not a failure.
func TestOfflineIsNotAnError(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	addNotes(t, db, 3)
	remote.offline = true

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("being offline was an error: %v", err)
	}
	if !stats.Offline {
		t.Error("the pass did not report being offline")
	}
	if stats.NotesPushed != 0 {
		t.Errorf("%d notes were pushed while offline", stats.NotesPushed)
	}

	// And the next pass carries what this one could not.
	remote.offline = false
	stats, err = w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 3 {
		t.Errorf("%d notes pushed once back online, want 3", stats.NotesPushed)
	}
}

// The cursor only moves after a push succeeded. A cursor moved first would
// lose the batch, with nothing left to say so.
func TestFailedPushKeepsTheCursor(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	addNotes(t, db, 4)
	remote.pushErr = errors.New("the far end is unhappy")

	if _, err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("a failing push was not reported")
	}

	cursor, err := db.Cursor(context.Background(), database.CursorNotes)
	if err != nil {
		t.Fatal(err)
	}
	if !cursor.IsZero() {
		t.Errorf("the cursor moved to %v despite the push failing", cursor)
	}

	// Nothing was lost: the same notes are still pending.
	remote.pushErr = nil
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 4 {
		t.Errorf("%d notes pushed on the retry, want 4", stats.NotesPushed)
	}
}

// A run interrupted halfway through a backlog keeps what it managed, so the
// next pass starts where it stopped rather than from the beginning.
func TestPartialRunKeepsItsProgress(t *testing.T) {
	w, db, remote := newWorker(t, Config{BatchSize: 2})
	addNotes(t, db, 6)

	// Two batches through, then the connection drops.
	remote.pushErr = nil
	pushed := 0
	remote.failOnce = false
	// Fail on the third push by swapping the error in after two.
	for pass := 0; pass < 2; pass++ {
		cursor, err := db.Cursor(context.Background(), database.CursorNotes)
		if err != nil {
			t.Fatal(err)
		}
		notes, next, err := db.PendingNotes(context.Background(), cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if err := remote.PushNotes(context.Background(), notes); err != nil {
			t.Fatal(err)
		}
		if err := db.SetCursor(context.Background(), database.CursorNotes, next); err != nil {
			t.Fatal(err)
		}
		pushed += len(notes)
	}
	if pushed != 4 {
		t.Fatalf("set-up pushed %d, want 4", pushed)
	}

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Only the two that were left, not all six again.
	if stats.NotesPushed != 2 {
		t.Errorf("%d notes pushed, want the 2 that were left", stats.NotesPushed)
	}
	if remote.count() != 6 {
		t.Errorf("the remote holds %d notes, want 6", remote.count())
	}
}

// A retried batch must not duplicate anything, which is why the interface
// requires the pushes to be idempotent.
func TestRetryDoesNotDuplicate(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	addNotes(t, db, 3)

	remote.failOnce = true
	if _, err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("the failing push was not reported")
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if remote.count() != 3 {
		t.Errorf("the remote holds %d notes after a retry, want 3", remote.count())
	}
}

// Notes before deletions. A note created and then deleted between two passes
// is already gone from the notes table, so the deletion removes an id the
// remote never had — which the interface requires to be harmless.
func TestDeletionsArePushed(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	ctx := context.Background()
	ids := addNotes(t, db, 3)

	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if remote.count() != 3 {
		t.Fatalf("the remote holds %d notes", remote.count())
	}

	if err := db.Delete(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.DeletionsPushed != 1 {
		t.Errorf("%d deletions pushed, want 1", stats.DeletionsPushed)
	}
	if remote.has(ids[1]) {
		t.Error("the deleted note is still on the remote")
	}
	if remote.count() != 2 {
		t.Errorf("the remote holds %d notes, want 2", remote.count())
	}
}

func TestNoteCreatedAndDeletedBetweenPasses(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	ctx := context.Background()

	note := &models.Note{ProjectID: "evomem", Content: "brief", SourceType: models.SourceManual}
	if err := db.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, note.ID); err != nil {
		t.Fatal(err)
	}

	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The row was gone before the pass, so it was never pushed; only the
	// tombstone travels.
	if stats.NotesPushed != 0 {
		t.Errorf("%d notes pushed, want 0", stats.NotesPushed)
	}
	if stats.DeletionsPushed != 1 {
		t.Errorf("%d deletions pushed, want 1", stats.DeletionsPushed)
	}
	if remote.count() != 0 {
		t.Errorf("the remote holds %d notes", remote.count())
	}
}

// An edited note goes again: updated_at moved past the cursor.
func TestEditedNoteIsPushedAgain(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	ctx := context.Background()

	note := &models.Note{ProjectID: "evomem", Content: "first", SourceType: models.SourceManual}
	if err := db.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	note.Content = "second"
	if err := db.Update(ctx, note); err != nil {
		t.Fatal(err)
	}
	stats, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NotesPushed != 1 {
		t.Fatalf("%d notes pushed after an edit, want 1", stats.NotesPushed)
	}

	remote.mu.Lock()
	got := remote.notes[note.ID].Content
	remote.mu.Unlock()
	if got != "second" {
		t.Errorf("the remote holds %q, want second", got)
	}
}

// Everything that reaches the remote carries its tainted mark, or the far end
// cannot tell third-party text from the user's own either.
func TestTaintedMarkTravels(t *testing.T) {
	w, db, remote := newWorker(t, Config{})
	ctx := context.Background()

	note := &models.Note{ProjectID: "evomem", Content: "from telegram", SourceType: models.SourceTelegram}
	note.MarkTainted("telegram")
	if err := db.Create(ctx, note); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	remote.mu.Lock()
	got := remote.notes[note.ID]
	remote.mu.Unlock()
	if got == nil || !got.Tainted() {
		t.Error("the tainted mark did not reach the remote")
	}
}

// --- the loop ---

// The loop has to stop when its context is cancelled, or nothing can
// supervise it.
func TestRunStopsOnCancel(t *testing.T) {
	w, db, _ := newWorker(t, Config{Interval: 10 * time.Millisecond})
	addNotes(t, db, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Run did not stop when its context was cancelled")
	}
}

func TestRunDrainsABacklog(t *testing.T) {
	w, db, remote := newWorker(t, Config{Interval: time.Hour, BatchSize: 2})
	addNotes(t, db, 7)

	// Interval is an hour, so if the loop waited between passes this
	// would time out. A backlog is worth draining promptly.
	drained := make(chan struct{})
	w.Observe(func(stats Stats, err error) {
		if err != nil {
			t.Errorf("a pass failed: %v", err)
		}
		if remote.count() == 7 {
			select {
			case <-drained:
			default:
				close(drained)
			}
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Errorf("the backlog was not drained: the remote holds %d of 7", remote.count())
	}
}

// A store syncing to a host that is down should not keep knocking at the
// ordinary interval forever.
func TestBackoffGrowsAndIsCapped(t *testing.T) {
	w, _, _ := newWorker(t, Config{Interval: time.Second, MaxBackoff: 10 * time.Second})

	// No failures: the ordinary interval, exactly, with no jitter.
	if got := w.backoff(0); got != time.Second {
		t.Errorf("backoff(0) = %v, want 1s", got)
	}

	previous := time.Duration(0)
	for failures := 1; failures <= 4; failures++ {
		got := w.backoff(failures)
		// Doubling until the cap, plus up to a quarter as jitter.
		want := time.Duration(1<<failures) * time.Second
		if want > w.cfg.MaxBackoff {
			want = w.cfg.MaxBackoff
		}
		if got < want || got > want+want/4+1 {
			t.Errorf("backoff(%d) = %v, want about %v", failures, got, want)
		}
		// Growth only holds below the cap; at the cap the jitter may
		// draw lower than the previous wait, which is the point of it.
		if want < w.cfg.MaxBackoff && got <= previous {
			t.Errorf("backoff(%d) = %v did not grow past %v", failures, got, previous)
		}
		previous = got
	}

	// Capped, jitter included.
	for _, failures := range []int{10, 100} {
		got := w.backoff(failures)
		if got > w.cfg.MaxBackoff+w.cfg.MaxBackoff/4+1 {
			t.Errorf("backoff(%d) = %v, above the cap %v", failures, got, w.cfg.MaxBackoff)
		}
	}
}

// Several machines syncing to one host fall into step after a shared outage
// and arrive together when it comes back. The jitter is what breaks that up.
func TestBackoffIsJittered(t *testing.T) {
	w, _, _ := newWorker(t, Config{Interval: time.Second, MaxBackoff: time.Minute})

	seen := map[time.Duration]bool{}
	for i := 0; i < 50; i++ {
		seen[w.backoff(3)] = true
	}
	if len(seen) < 5 {
		t.Errorf("50 draws gave %d distinct waits; the jitter is not working", len(seen))
	}
}

func TestObserveSeesEveryPass(t *testing.T) {
	w, db, _ := newWorker(t, Config{})
	addNotes(t, db, 1)

	var got Stats
	w.Observe(func(stats Stats, err error) { got = stats })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if got.NotesPushed != 1 {
		t.Errorf("the observer saw %+v", got)
	}
	if got.Duration == 0 {
		t.Error("the pass reported no duration")
	}
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
