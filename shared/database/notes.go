package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// ErrNotFound is returned when an identifier matches no note.
var ErrNotFound = errors.New("database: no such note")

// timeLayout is how a timestamp is stored. SQLite has no date type, so the
// format has to sort correctly as text: RFC 3339 in UTC with a fixed number of
// fractional digits does, and a local offset would not.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// defaultLimit bounds a query that did not ask for a bound. A note store grows
// without limit and a caller that forgot to say how much it wanted should not
// be handed all of it.
const defaultLimit = 50

// maxLimit bounds one the caller did ask for.
const maxLimit = 1000

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		// Tolerated for rows written by hand or by an older format:
		// the column is DATETIME and SQLite accepts anything in it.
		if t2, err2 := time.Parse(time.RFC3339Nano, s); err2 == nil {
			return t2.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("database: unreadable timestamp %q: %w", s, err)
	}
	return t, nil
}

// Create stores a note, filling in the identifier and timestamps the caller
// left empty. The note is updated in place so the caller has the identifier
// that was assigned.
func (d *DB) Create(ctx context.Context, n *models.Note) error {
	if n == nil {
		return errors.New("database: no note given")
	}
	if err := n.Validate(); err != nil {
		return err
	}

	if n.ID == "" {
		n.ID = models.NewULID()
	} else {
		id, err := models.NormalizeULID(n.ID)
		if err != nil {
			return err
		}
		n.ID = id
	}

	now := time.Now().UTC()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = n.CreatedAt
	}

	metadata, err := n.MarshalMetadata()
	if err != nil {
		return err
	}

	_, err = d.write.ExecContext(ctx, `
		INSERT INTO notes (id, project_id, content, source_type, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.ProjectID, n.Content, string(n.SourceType),
		formatTime(n.CreatedAt), formatTime(n.UpdatedAt), metadata)
	if err != nil {
		return fmt.Errorf("database: creating note %s: %w", n.ID, err)
	}
	return nil
}

// CreateBatch stores many notes in one transaction. Ingestion arrives in
// batches — a Jira page, a day of Telegram messages — and one transaction per
// note would mean one fsync per note.
func (d *DB) CreateBatch(ctx context.Context, notes []*models.Note) error {
	if len(notes) == 0 {
		return nil
	}
	for _, n := range notes {
		if n == nil {
			return errors.New("database: nil note in batch")
		}
		if err := n.Validate(); err != nil {
			return err
		}
	}

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: starting a batch: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO notes (id, project_id, content, source_type, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("database: preparing a batch: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, n := range notes {
		if n.ID == "" {
			n.ID = models.NewULID()
		} else {
			id, err := models.NormalizeULID(n.ID)
			if err != nil {
				return err
			}
			n.ID = id
		}
		if n.CreatedAt.IsZero() {
			n.CreatedAt = now
		}
		if n.UpdatedAt.IsZero() {
			n.UpdatedAt = n.CreatedAt
		}
		metadata, err := n.MarshalMetadata()
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			n.ID, n.ProjectID, n.Content, string(n.SourceType),
			formatTime(n.CreatedAt), formatTime(n.UpdatedAt), metadata); err != nil {
			return fmt.Errorf("database: creating note %s: %w", n.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: committing a batch: %w", err)
	}
	return nil
}

const noteColumns = `id, project_id, content, source_type, created_at, updated_at, metadata`

// scanNote reads one row of noteColumns.
func scanNote(s interface{ Scan(...any) error }) (*models.Note, error) {
	var (
		n          models.Note
		sourceType string
		created    string
		updated    string
		metadata   string
	)
	if err := s.Scan(&n.ID, &n.ProjectID, &n.Content, &sourceType, &created, &updated, &metadata); err != nil {
		return nil, err
	}

	n.SourceType = models.SourceType(sourceType)

	var err error
	if n.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if n.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	if err := n.UnmarshalMetadata(metadata); err != nil {
		return nil, err
	}
	return &n, nil
}

// Get returns one note by identifier.
func (d *DB) Get(ctx context.Context, id string) (*models.Note, error) {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return nil, err
	}

	row := d.read.QueryRowContext(ctx, `SELECT `+noteColumns+` FROM notes WHERE id = ?`, id)
	n, err := scanNote(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("database: reading note %s: %w", id, err)
	}
	return n, nil
}

// Update replaces the content and the metadata of an existing note and moves
// its updated_at forward. The identifier, the project and the source are what
// the note is, not what it currently says, and are left alone.
func (d *DB) Update(ctx context.Context, n *models.Note) error {
	if n == nil {
		return errors.New("database: no note given")
	}
	if err := n.Validate(); err != nil {
		return err
	}
	id, err := models.NormalizeULID(n.ID)
	if err != nil {
		return err
	}
	n.ID = id

	metadata, err := n.MarshalMetadata()
	if err != nil {
		return err
	}
	n.UpdatedAt = time.Now().UTC()

	res, err := d.write.ExecContext(ctx, `
		UPDATE notes SET content = ?, metadata = ?, updated_at = ? WHERE id = ?`,
		n.Content, metadata, formatTime(n.UpdatedAt), n.ID)
	if err != nil {
		return fmt.Errorf("database: updating note %s: %w", n.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("database: updating note %s: %w", n.ID, err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, n.ID)
	}
	return nil
}

// Delete removes one note and leaves a tombstone.
//
// The tombstone is the point. Once the row is gone there is nothing left to
// tell the far end it went: a delete would be indistinguishable from a note
// that was never there, and the cloud copy would outlive the local one
// forever. It is written in the same transaction as the delete, so the two
// cannot come apart.
//
// A note that was not there is reported as ErrNotFound rather than silently
// accepted, because a caller deleting by an identifier it was given wants to
// know the identifier was wrong.
func (d *DB) Delete(ctx context.Context, id string) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: deleting note %s: %w", id, err)
	}
	defer tx.Rollback()

	var projectID string
	err = tx.QueryRowContext(ctx, `SELECT project_id FROM notes WHERE id = ?`, id).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("database: deleting note %s: %w", id, err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("database: deleting note %s: %w", id, err)
	}
	// INSERT OR REPLACE, so deleting an identifier that was deleted
	// before — possible after a restore — moves the tombstone forward
	// rather than failing on the primary key.
	if _, err := tx.ExecContext(ctx,
		tx.dialect.insertOrReplace("deletions",
			"id, project_id, deleted_at", "?, ?, ?",
			"id", "project_id = EXCLUDED.project_id, deleted_at = EXCLUDED.deleted_at"),
		id, projectID, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("database: recording the deletion of %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: deleting note %s: %w", id, err)
	}
	// After the commit: the row is gone for certain, so the recording
	// should be too. ADR-0018 keeps recordings after transcription, which
	// is only defensible if a delete reaches them.
	return d.removeRecording(id)
}

// ListOptions bounds a listing. A zero Limit means defaultLimit.
type ListOptions struct {
	ProjectID  string
	SourceType models.SourceType
	Limit      int
	Offset     int
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > maxLimit:
		return maxLimit
	default:
		return limit
	}
}

// List returns notes newest first, optionally narrowed to one project or one
// source.
func (d *DB) List(ctx context.Context, opts ListOptions) ([]*models.Note, error) {
	query := `SELECT ` + noteColumns + ` FROM notes`
	var (
		where []string
		args  []any
	)
	if opts.ProjectID != "" {
		where = append(where, `project_id = ?`)
		args = append(args, opts.ProjectID)
	}
	if opts.SourceType != "" {
		where = append(where, `source_type = ?`)
		args = append(args, string(opts.SourceType))
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	// id breaks a tie on created_at, and because it is a ULID that tie is
	// broken in creation order rather than arbitrarily.
	query += ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, clampLimit(opts.Limit), max(0, opts.Offset))

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: listing notes: %w", err)
	}
	defer rows.Close()

	var out []*models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing notes: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: listing notes: %w", err)
	}
	return out, nil
}

// Count returns how many notes match, ignoring Limit and Offset. A caller
// paging through a project needs to know when to stop.
func (d *DB) Count(ctx context.Context, opts ListOptions) (int, error) {
	query := `SELECT COUNT(*) FROM notes`
	var (
		where []string
		args  []any
	)
	if opts.ProjectID != "" {
		where = append(where, `project_id = ?`)
		args = append(args, opts.ProjectID)
	}
	if opts.SourceType != "" {
		where = append(where, `source_type = ?`)
		args = append(args, string(opts.SourceType))
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}

	var count int
	if err := d.read.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("database: counting notes: %w", err)
	}
	return count, nil
}

// Projects returns every project that has notes, with how many and when the
// newest one was written.
func (d *DB) Projects(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := d.read.QueryContext(ctx, `
		SELECT project_id, COUNT(*), MAX(created_at)
		FROM notes GROUP BY project_id ORDER BY MAX(created_at) DESC`)
	if err != nil {
		return nil, fmt.Errorf("database: listing projects: %w", err)
	}
	defer rows.Close()

	var out []ProjectSummary
	for rows.Next() {
		var (
			s      ProjectSummary
			newest string
		)
		if err := rows.Scan(&s.ProjectID, &s.Notes, &newest); err != nil {
			return nil, fmt.Errorf("database: listing projects: %w", err)
		}
		if s.NewestAt, err = parseTime(newest); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: listing projects: %w", err)
	}
	return out, nil
}

// ProjectSummary is one row of Projects.
type ProjectSummary struct {
	ProjectID string    `json:"project_id"`
	Notes     int       `json:"notes"`
	NewestAt  time.Time `json:"newest_at"`
}

// DeleteAllNotes removes all notes from the database. Used for restore.
func (d *DB) DeleteAllNotes(ctx context.Context) error {
	if _, err := d.write.ExecContext(ctx, `DELETE FROM notes`); err != nil {
		return err
	}
	// Every note is gone, so every recording goes with it. A restore that
	// kept them would leave recordings belonging to notes that no longer
	// exist, under identifiers the new rows may reuse.
	if d.files == nil {
		return nil
	}
	if err := d.files.RemoveAll(); err != nil {
		return fmt.Errorf("database: the notes are deleted but their recordings are not: %w", err)
	}
	return nil
}

// GetAllNotes returns all notes with optional pagination. Used for restore.
func (d *DB) GetAllNotes(ctx context.Context, limit, offset int) ([]*models.Note, error) {
	query := `SELECT ` + noteColumns + ` FROM notes ORDER BY created_at ASC LIMIT ? OFFSET ?`
	rows, err := d.read.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("database: getting all notes: %w", err)
	}
	defer rows.Close()

	var out []*models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("database: getting all notes: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: getting all notes: %w", err)
	}
	return out, nil
}

// AwaitingTranscription returns notes that stand for a recording nobody has
// transcribed yet, oldest first.
//
// Oldest first, unlike every other listing here: this is a queue being worked
// through, and a recording that has waited longest should not keep losing to
// whatever arrived this morning.
//
// The mark lives in the metadata column rather than in a column of its own,
// because a note awaiting a transcript is a passing state of one source's
// notes and not a pillar of the schema. json_extract reads it; the predicate
// is `= 1` because SQLite's JSON true is the integer 1.
func (d *DB) AwaitingTranscription(ctx context.Context, limit int) ([]*models.Note, error) {
	rows, err := d.read.QueryContext(ctx, `SELECT `+noteColumns+` FROM notes
		WHERE `+d.read.dialect.metadataIsTrue(models.MetaAwaitingTranscription)+`
		ORDER BY created_at ASC, id ASC LIMIT ?`, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("database: listing notes awaiting transcription: %w", err)
	}
	defer rows.Close()

	var out []*models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing notes awaiting transcription: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: listing notes awaiting transcription: %w", err)
	}
	return out, nil
}

// AwaitingTranscriptionCount is how many are waiting, for a status line.
func (d *DB) AwaitingTranscriptionCount(ctx context.Context) (int, error) {
	var n int
	if err := d.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes
		WHERE `+d.read.dialect.metadataIsTrue(models.MetaAwaitingTranscription)).Scan(&n); err != nil {
		return 0, fmt.Errorf("database: counting notes awaiting transcription: %w", err)
	}
	return n, nil
}
