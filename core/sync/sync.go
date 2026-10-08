// Package sync pushes local changes to a remote copy of the store.
//
// It is one-directional on purpose: the local SQLite file is the authority
// and the remote is a mirror. Two-way merge would need per-field conflict
// resolution and a clock nobody trusts, and nothing in the product asks for
// it — a note is written once, in one place, and rarely edited.
package sync

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Remote is the far end of a sync. A transport implements it; this package
// does not care which.
//
// Every method must be idempotent. A batch whose push failed halfway is
// retried whole, because the cursor only moves after the whole batch
// succeeded, so a remote that cannot take the same note twice will duplicate
// it on the first flaky connection.
type Remote interface {
	// Ping reports whether the remote can be reached and written to now.
	//
	// This is what "is there a connection" means here. A probe to some
	// third party would answer a different question, tell the third party
	// that this user is online, and still not say whether the one host
	// that matters is up.
	Ping(ctx context.Context) error

	// PushNotes upserts notes by id.
	PushNotes(ctx context.Context, notes []*models.Note) error

	// PushDeletions removes notes by id. An id that is not there is not an
	// error: the local side may be reporting a note the remote never saw.
	PushDeletions(ctx context.Context, deletions []database.Deletion) error

	// PullNotes fetches notes from the remote that are newer than the cursor.
	// Returns notes and the new cursor position.
	PullNotes(ctx context.Context, cursorUpdatedAt, cursorID string, limit int) ([]*models.Note, string, string, error)

	// PullAll fetches all notes from the remote (for restore).
	PullAll(ctx context.Context, limit int, offset int) ([]*models.Note, error)

	// Close releases the connection.
	Close() error
}

// Defaults for a worker that was not told otherwise.
const (
	DefaultInterval   = 5 * time.Minute
	DefaultBatchSize  = 200
	DefaultMaxBackoff = 30 * time.Minute
)

// Config tunes a Worker.
type Config struct {
	// Interval is how long to wait between passes that found nothing.
	Interval time.Duration

	// BatchSize is how many notes or deletions go in one push.
	BatchSize int

	// MaxBackoff caps the wait after repeated failures.
	MaxBackoff time.Duration
}

func (c *Config) setDefaults() {
	if c.Interval <= 0 {
		c.Interval = DefaultInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = DefaultMaxBackoff
	}
}

// Stats is what one pass did.
type Stats struct {
	NotesPushed     int           `json:"notes_pushed"`
	DeletionsPushed int           `json:"deletions_pushed"`
	Batches         int           `json:"batches"`
	Duration        time.Duration `json:"duration"`

	// Offline says the remote could not be reached. It is not an error:
	// a laptop in a tunnel is the normal case for a local-first store,
	// and the next pass will carry what this one could not.
	Offline bool `json:"offline"`
}

// Empty reports whether the pass had nothing to do.
func (s Stats) Empty() bool { return s.NotesPushed == 0 && s.DeletionsPushed == 0 }

// Worker pushes changes to a remote, once or on a loop.
type Worker struct {
	db     *database.DB
	remote Remote
	cfg    Config

	// observe is called after every pass. A worker in a daemon has
	// nowhere to print to, and a worker in a test needs to see what
	// happened without reading a log.
	observe func(Stats, error)
}

// New returns a worker.
func New(db *database.DB, remote Remote, cfg Config) (*Worker, error) {
	if db == nil {
		return nil, errors.New("sync: no store")
	}
	if remote == nil {
		return nil, errors.New("sync: no remote")
	}
	cfg.setDefaults()
	return &Worker{db: db, remote: remote, cfg: cfg}, nil
}

// Observe sets a callback run after every pass.
func (w *Worker) Observe(f func(Stats, error)) { w.observe = f }

// RunOnce makes one pass and returns what it did.
//
// Notes before deletions. A note created and then deleted between two passes
// is already gone from the notes table, so it is never pushed, and the
// deletion that follows removes an id the remote never had — which the
// interface requires to be harmless. The other order would risk pushing a
// note after the delete that was meant to remove it.
func (w *Worker) RunOnce(ctx context.Context) (Stats, error) {
	started := time.Now()
	var stats Stats

	if err := w.remote.Ping(ctx); err != nil {
		stats.Offline = true
		stats.Duration = time.Since(started)
		return stats, nil
	}

	if err := w.pushNotes(ctx, &stats); err != nil {
		stats.Duration = time.Since(started)
		return stats, err
	}
	if err := w.pushDeletions(ctx, &stats); err != nil {
		stats.Duration = time.Since(started)
		return stats, err
	}

	stats.Duration = time.Since(started)
	return stats, nil
}

// pushNotes sends changed notes in batches, moving the cursor after each one.
//
// After each batch, not at the end: a run interrupted halfway through a large
// backlog keeps what it managed, and the next pass starts where it stopped
// rather than from the beginning.
func (w *Worker) pushNotes(ctx context.Context, stats *Stats) error {
	for {
		cursor, err := w.db.Cursor(ctx, database.CursorNotes)
		if err != nil {
			return err
		}
		notes, next, err := w.db.PendingNotes(ctx, cursor, w.cfg.BatchSize)
		if err != nil {
			return err
		}
		if len(notes) == 0 {
			return nil
		}

		if err := w.remote.PushNotes(ctx, notes); err != nil {
			return fmt.Errorf("sync: pushing %d note(s): %w", len(notes), err)
		}
		// Only now. A cursor moved before the push would lose the batch
		// on a failure, and there would be nothing left to say so.
		if err := w.db.SetCursor(ctx, database.CursorNotes, next); err != nil {
			return err
		}

		stats.NotesPushed += len(notes)
		stats.Batches++

		if len(notes) < w.cfg.BatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (w *Worker) pushDeletions(ctx context.Context, stats *Stats) error {
	for {
		cursor, err := w.db.Cursor(ctx, database.CursorDeletions)
		if err != nil {
			return err
		}
		deletions, next, err := w.db.PendingDeletions(ctx, cursor, w.cfg.BatchSize)
		if err != nil {
			return err
		}
		if len(deletions) == 0 {
			return nil
		}

		if err := w.remote.PushDeletions(ctx, deletions); err != nil {
			return fmt.Errorf("sync: pushing %d deletion(s): %w", len(deletions), err)
		}
		if err := w.db.SetCursor(ctx, database.CursorDeletions, next); err != nil {
			return err
		}

		stats.DeletionsPushed += len(deletions)
		stats.Batches++

		if len(deletions) < w.cfg.BatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// Run makes passes until ctx is cancelled.
//
// A pass that failed or found the remote unreachable waits longer than the
// next one would: a store syncing to a host that is down should not keep
// knocking every five minutes forever, and a laptop that is offline all day
// should not spend the day retrying. A pass that did something resets the
// wait, because a backlog is worth draining promptly.
func (w *Worker) Run(ctx context.Context) error {
	failures := 0

	for {
		stats, err := w.RunOnce(ctx)
		if w.observe != nil {
			w.observe(stats, err)
		}

		switch {
		case err != nil, stats.Offline:
			failures++
		default:
			failures = 0
		}

		// More work waiting, and the last pass went through: carry on
		// without waiting.
		if err == nil && !stats.Offline && !stats.Empty() {
			if more, err := w.hasPending(ctx); err == nil && more {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
		}

		wait := w.backoff(failures)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (w *Worker) hasPending(ctx context.Context) (bool, error) {
	notes, deletions, err := w.db.PendingCount(ctx)
	if err != nil {
		return false, err
	}
	return notes+deletions > 0, nil
}

// backoff is the interval doubled per consecutive failure, capped, with up to
// a quarter of the wait added at random.
//
// The jitter is not decoration. Several machines syncing to one host tend to
// fall into step after a shared outage, and arrive together when it comes
// back.
func (w *Worker) backoff(failures int) time.Duration {
	if failures <= 0 {
		return w.cfg.Interval
	}

	wait := w.cfg.Interval
	for i := 0; i < failures && wait < w.cfg.MaxBackoff; i++ {
		wait *= 2
	}
	if wait > w.cfg.MaxBackoff {
		wait = w.cfg.MaxBackoff
	}
	return wait + rand.N(wait/4+1)
}

// Pull pulls notes from the remote that are newer than the local cursor.
// Returns the new cursor position.
func (w *Worker) Pull(ctx context.Context) (database.Cursor, error) {
	cursor, err := w.db.Cursor(ctx, database.CursorNotes)
	if err != nil {
		return database.Cursor{}, err
	}

	notes, nextUpdatedAtStr, nextID, err := w.remote.PullNotes(ctx, cursor.UpdatedAt.Format(time.RFC3339), cursor.ID, w.cfg.BatchSize)
	if err != nil {
		return database.Cursor{}, err
	}

	for _, note := range notes {
		if err := w.db.Create(ctx, note); err != nil {
			return database.Cursor{}, fmt.Errorf("pull: creating note %s: %w", note.ID, err)
		}
	}

	nextUpdatedAt, err := time.Parse(time.RFC3339Nano, nextUpdatedAtStr)
	if err != nil {
		return database.Cursor{}, fmt.Errorf("pull: parsing cursor time: %w", err)
	}

	next := database.Cursor{UpdatedAt: nextUpdatedAt, ID: nextID}

	if err := w.db.SetCursor(ctx, database.CursorNotes, next); err != nil {
		return database.Cursor{}, err
	}

	return next, nil
}

// Restore replaces all local notes with the remote copy.
// This is a destructive operation: all local notes are deleted first.
func (w *Worker) Restore(ctx context.Context) error {
	// Delete all local notes
	if err := w.db.DeleteAllNotes(ctx); err != nil {
		return fmt.Errorf("restore: clearing local notes: %w", err)
	}

	// Reset cursor
	if err := w.db.SetCursor(ctx, database.CursorNotes, database.Cursor{}); err != nil {
		return fmt.Errorf("restore: resetting cursor: %w", err)
	}

	// Pull all notes in batches
	offset := 0
	for {
		notes, err := w.remote.PullAll(ctx, 1000, offset)
		if err != nil {
			return fmt.Errorf("restore: pulling notes: %w", err)
		}
		if len(notes) == 0 {
			break
		}
		for _, note := range notes {
			if err := w.db.Create(ctx, note); err != nil {
				return fmt.Errorf("restore: creating note %s: %w", note.ID, err)
			}
		}
		offset += len(notes)
		if len(notes) < 1000 {
			break
		}
	}

	// Update cursor to end
	return nil
}
