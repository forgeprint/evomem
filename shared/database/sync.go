package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// The watermarks the sync engine keeps. One row each in sync_state, so a new
// kind of watermark does not need a migration.
const (
	// CursorNotes is how far the engine has got through changed notes.
	CursorNotes = "notes"
	// CursorDeletions is how far it has got through tombstones.
	CursorDeletions = "deletions"
)

// A Cursor is a position in the (updated_at, id) order.
//
// The pair rather than the timestamp alone: two notes written in the same
// millisecond would otherwise be indistinguishable, and a cursor that could
// only say "this millisecond" has to either re-send one of them or skip it.
// The id breaks the tie, and because it is a ULID it breaks it in creation
// order.
type Cursor struct {
	UpdatedAt time.Time
	ID        string
}

// IsZero reports whether nothing has been synced yet.
func (c Cursor) IsZero() bool { return c.ID == "" && c.UpdatedAt.IsZero() }

// String is for a log line, not for storage.
func (c Cursor) String() string {
	if c.IsZero() {
		return "(nothing synced)"
	}
	return c.UpdatedAt.Format(time.RFC3339) + " " + c.ID
}

// A Deletion is a note the user removed. It carries the project so the far
// end can scope the delete without having to know the note.
type Deletion struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

// Cursor reads one watermark. A key that was never written is the zero
// cursor, which means everything is pending.
func (d *DB) Cursor(ctx context.Context, key string) (Cursor, error) {
	var at, id string
	err := d.read.QueryRowContext(ctx,
		`SELECT value FROM sync_state WHERE key = ?`, key+".at").Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return Cursor{}, nil
	}
	if err != nil {
		return Cursor{}, fmt.Errorf("database: reading the %s cursor: %w", key, err)
	}
	if err := d.read.QueryRowContext(ctx,
		`SELECT value FROM sync_state WHERE key = ?`, key+".id").Scan(&id); err != nil &&
		!errors.Is(err, sql.ErrNoRows) {
		return Cursor{}, fmt.Errorf("database: reading the %s cursor: %w", key, err)
	}

	parsed, err := parseTime(at)
	if err != nil {
		return Cursor{}, err
	}
	return Cursor{UpdatedAt: parsed, ID: id}, nil
}

// SetCursor moves one watermark forward.
//
// It refuses to move one backwards. Re-sending a note is harmless — the far
// end upserts — but a cursor that went back would re-send everything after it
// too, and a bug that quietly re-uploads the whole store on every pass is the
// kind that is noticed by the bill.
func (d *DB) SetCursor(ctx context.Context, key string, c Cursor) error {
	if c.IsZero() {
		return errors.New("database: refusing to set an empty cursor")
	}

	current, err := d.Cursor(ctx, key)
	if err != nil {
		return err
	}
	if !current.IsZero() && !after(c, current) {
		return fmt.Errorf("database: the %s cursor is at %s; refusing to move it back to %s",
			key, current, c)
	}

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: setting the %s cursor: %w", key, err)
	}
	defer tx.Rollback()

	for suffix, value := range map[string]string{".at": formatTime(c.UpdatedAt), ".id": c.ID} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sync_state (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key+suffix, value); err != nil {
			return fmt.Errorf("database: setting the %s cursor: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: setting the %s cursor: %w", key, err)
	}
	return nil
}

// after reports whether a is past b in the (updated_at, id) order.
func after(a, b Cursor) bool {
	if a.UpdatedAt.After(b.UpdatedAt) {
		return true
	}
	if a.UpdatedAt.Before(b.UpdatedAt) {
		return false
	}
	return a.ID > b.ID
}

// PendingNotes returns notes changed since the cursor, oldest change first,
// together with the cursor that covers what was returned.
//
// Oldest first, not newest: the caller sends a batch and then moves the
// cursor, so the order has to be the one in which a partial send still leaves
// a correct watermark.
func (d *DB) PendingNotes(ctx context.Context, from Cursor, limit int) ([]*models.Note, Cursor, error) {
	limit = clampLimit(limit)

	query := `SELECT ` + noteColumns + ` FROM notes`
	var args []any
	if !from.IsZero() {
		// The row-value comparison SQLite supports, which uses the
		// (updated_at, id) index rather than scanning.
		query += ` WHERE (updated_at, id) > (?, ?)`
		args = append(args, formatTime(from.UpdatedAt), from.ID)
	}
	query += ` ORDER BY updated_at, id LIMIT ?`
	args = append(args, limit)

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, from, fmt.Errorf("database: reading pending notes: %w", err)
	}
	defer rows.Close()

	out := []*models.Note{}
	next := from
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, from, fmt.Errorf("database: reading pending notes: %w", err)
		}
		out = append(out, n)
		next = Cursor{UpdatedAt: n.UpdatedAt, ID: n.ID}
	}
	if err := rows.Err(); err != nil {
		return nil, from, fmt.Errorf("database: reading pending notes: %w", err)
	}
	return out, next, nil
}

// PendingDeletions returns tombstones since the cursor, oldest first.
func (d *DB) PendingDeletions(ctx context.Context, from Cursor, limit int) ([]Deletion, Cursor, error) {
	limit = clampLimit(limit)

	query := `SELECT id, project_id, deleted_at FROM deletions`
	var args []any
	if !from.IsZero() {
		query += ` WHERE (deleted_at, id) > (?, ?)`
		args = append(args, formatTime(from.UpdatedAt), from.ID)
	}
	query += ` ORDER BY deleted_at, id LIMIT ?`
	args = append(args, limit)

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, from, fmt.Errorf("database: reading pending deletions: %w", err)
	}
	defer rows.Close()

	out := []Deletion{}
	next := from
	for rows.Next() {
		var (
			del Deletion
			at  string
		)
		if err := rows.Scan(&del.ID, &del.ProjectID, &at); err != nil {
			return nil, from, fmt.Errorf("database: reading pending deletions: %w", err)
		}
		if del.DeletedAt, err = parseTime(at); err != nil {
			return nil, from, err
		}
		out = append(out, del)
		next = Cursor{UpdatedAt: del.DeletedAt, ID: del.ID}
	}
	if err := rows.Err(); err != nil {
		return nil, from, fmt.Errorf("database: reading pending deletions: %w", err)
	}
	return out, next, nil
}

// PendingCount is how much is waiting, for a status line. It is two counts
// rather than one because a store with nothing new and a thousand unsent
// deletes is not idle.
func (d *DB) PendingCount(ctx context.Context) (notes, deletions int, err error) {
	noteCursor, err := d.Cursor(ctx, CursorNotes)
	if err != nil {
		return 0, 0, err
	}
	deleteCursor, err := d.Cursor(ctx, CursorDeletions)
	if err != nil {
		return 0, 0, err
	}

	if noteCursor.IsZero() {
		err = d.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes`).Scan(&notes)
	} else {
		err = d.read.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM notes WHERE (updated_at, id) > (?, ?)`,
			formatTime(noteCursor.UpdatedAt), noteCursor.ID).Scan(&notes)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("database: counting pending notes: %w", err)
	}

	if deleteCursor.IsZero() {
		err = d.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM deletions`).Scan(&deletions)
	} else {
		err = d.read.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM deletions WHERE (deleted_at, id) > (?, ?)`,
			formatTime(deleteCursor.UpdatedAt), deleteCursor.ID).Scan(&deletions)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("database: counting pending deletions: %w", err)
	}
	return notes, deletions, nil
}

// ArchiveOptions bounds what Archive removes.
type ArchiveOptions struct {
	// Before removes notes created strictly before this instant. A zero
	// value removes nothing: there is no default age, because the default
	// would be a number nobody chose deleting the user's notes.
	Before time.Time

	// ProjectID narrows the archiving to one project.
	ProjectID string

	// IncludeUnsynced removes notes that have not been synced yet.
	//
	// Off by default, and the default is the whole point. Archiving is
	// meant to thin a local file whose contents exist in the cloud; a
	// note past the sync cursor exists only here, and removing it is not
	// archiving but deleting. A store that was never synced therefore
	// archives nothing until the caller says, in as many words, that it
	// means to lose the data.
	IncludeUnsynced bool

	// Vacuum rewrites the file afterwards, which is what actually returns
	// the space to the filesystem. A delete alone leaves free pages
	// inside the file.
	Vacuum bool
}

// ArchiveResult says what Archive did.
type ArchiveResult struct {
	// NotesRemoved and TombstonesRemoved are what was deleted.
	NotesRemoved      int `json:"notes_removed"`
	TombstonesRemoved int `json:"tombstones_removed"`

	// Skipped is how many notes matched the age but were left because
	// they had not been synced and IncludeUnsynced was off. It is the
	// number that explains an archive run that seemed to do nothing.
	Skipped int `json:"skipped_unsynced"`

	// BytesBefore and BytesAfter are the file's size either side of a
	// vacuum, and are zero when Vacuum was off.
	BytesBefore int64 `json:"bytes_before,omitempty"`
	BytesAfter  int64 `json:"bytes_after,omitempty"`
}

// Archive removes old notes to thin the local file.
//
// Deletions here leave no tombstone. A tombstone means "the user removed
// this", and the far end would act on it by deleting the cloud copy — which
// is the one copy archiving is relying on. The two look the same in SQL and
// mean opposite things, which is why Delete writes the tombstone explicitly
// rather than a trigger doing it for both.
//
// Synced tombstones older than Before are removed too. Otherwise the
// deletions table is the one thing in the store that only ever grows.
func (d *DB) Archive(ctx context.Context, opts ArchiveOptions) (ArchiveResult, error) {
	var result ArchiveResult
	if opts.Before.IsZero() {
		return result, errors.New("database: archiving needs a cutoff")
	}
	cutoff := formatTime(opts.Before.UTC())

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("database: archiving: %w", err)
	}
	defer tx.Rollback()

	if err := d.archiveNotes(ctx, tx, cutoff, opts, &result); err != nil {
		return result, err
	}
	// Independently of the notes. A store that has synced deletions but
	// has no note cursor yet still has tombstones worth clearing, and an
	// early return above would have left them to grow forever.
	if err := d.archiveTombstones(ctx, tx, cutoff, &result); err != nil {
		return result, err
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("database: archiving: %w", err)
	}

	if opts.Vacuum {
		if err := d.vacuum(ctx, &result); err != nil {
			// The rows are gone and that is committed; only the
			// space was not returned.
			return result, err
		}
	}
	return result, nil
}

// archiveNotes removes old notes, holding back any the far end has not seen.
func (d *DB) archiveNotes(ctx context.Context, tx *sql.Tx, cutoff string,
	opts ArchiveOptions, result *ArchiveResult) error {

	where, args := archiveWhere(cutoff, opts.ProjectID)

	// What would go if nothing were held back, so the caller can be told
	// why a run that looked like a no-op was one.
	var matched int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes `+where, args...).Scan(&matched); err != nil {
		return fmt.Errorf("database: counting what would be archived: %w", err)
	}

	if !opts.IncludeUnsynced {
		cursor, err := d.Cursor(ctx, CursorNotes)
		if err != nil {
			return err
		}
		if cursor.IsZero() {
			// Nothing has ever been synced, so every note exists
			// only here and none of them may be removed.
			result.Skipped = matched
			return nil
		}
		where += ` AND (updated_at, id) <= (?, ?)`
		args = append(args, formatTime(cursor.UpdatedAt), cursor.ID)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM notes `+where, args...)
	if err != nil {
		return fmt.Errorf("database: archiving: %w", err)
	}
	removed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("database: archiving: %w", err)
	}
	result.NotesRemoved = int(removed)
	result.Skipped = matched - result.NotesRemoved
	return nil
}

// archiveTombstones removes tombstones the far end already knows about and
// that are older than the cutoff: they have nothing left to tell anyone.
//
// An unsynced tombstone is kept whatever IncludeUnsynced says. Dropping one
// would leave the cloud copy of a note the user deleted in place forever,
// with nothing anywhere to say it should go.
func (d *DB) archiveTombstones(ctx context.Context, tx *sql.Tx, cutoff string, result *ArchiveResult) error {
	cursor, err := d.Cursor(ctx, CursorDeletions)
	if err != nil {
		return err
	}
	if cursor.IsZero() {
		return nil
	}

	res, err := tx.ExecContext(ctx,
		`DELETE FROM deletions WHERE deleted_at < ? AND (deleted_at, id) <= (?, ?)`,
		cutoff, formatTime(cursor.UpdatedAt), cursor.ID)
	if err != nil {
		return fmt.Errorf("database: archiving tombstones: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil {
		result.TombstonesRemoved = int(n)
	}
	return nil
}

func archiveWhere(cutoff, projectID string) (string, []any) {
	// created_at, not updated_at: the age of a record is when it was
	// written, and editing an old note does not make it new.
	where := `WHERE created_at < ?`
	args := []any{cutoff}
	if projectID != "" {
		where += ` AND project_id = ?`
		args = append(args, projectID)
	}
	return where, args
}

// vacuum rewrites the file, which is what returns the space a delete only
// marked as free.
//
// This is where ADR-0005's explicit rowid earns itself: VACUUM may renumber
// an implicit rowid, and the full-text index is joined to notes by rowid.
// With an INTEGER PRIMARY KEY the numbering is stable and the index survives.
func (d *DB) vacuum(ctx context.Context, result *ArchiveResult) error {
	result.BytesBefore = d.fileSize(ctx)

	// VACUUM cannot run inside a transaction, and it needs the write pool
	// to itself — which the single connection guarantees.
	if _, err := d.write.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("database: vacuuming: %w", err)
	}
	result.BytesAfter = d.fileSize(ctx)
	return nil
}

// fileSize asks SQLite rather than the filesystem, so it is the same answer
// for a file and for an in-memory store.
func (d *DB) fileSize(ctx context.Context) int64 {
	var pageCount, pageSize int64
	if err := d.read.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0
	}
	if err := d.read.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0
	}
	return pageCount * pageSize
}
