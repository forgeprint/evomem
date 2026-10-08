// Postgres is the one transport this build ships. It implements Remote over
// database/sql with the pgx driver in its standard-library mode.
//
// The driver's import path and registered name were checked against
// pkg.go.dev on 2026-10-06: github.com/jackc/pgx/v5/stdlib registers "pgx".
// See ADR-0011 for why this is the second dependency.

package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

// driverName is what pgx registers with database/sql.
const driverName = "pgx"

// pingTimeout bounds a reachability check. It has to be short: the worker
// asks this question on a laptop that may be on a hotel network, and a probe
// that hangs for thirty seconds makes every pass feel like a failure.
const pingTimeout = 5 * time.Second

// Postgres is a remote copy of the store in a PostgreSQL database.
type Postgres struct {
	db *sql.DB
}

// OpenPostgres connects to a PostgreSQL database.
//
// It does not create the schema. Running DDL against someone's database
// because a process started is not a thing a sync client should decide;
// EnsureSchema is explicit, and Ping says plainly when the tables are not
// there yet.
func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("sync: no PostgreSQL connection string")
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sync: opening PostgreSQL: %w", err)
	}
	// A sync client is one background worker making one batch at a time.
	// A larger pool would hold connections open on the far end for
	// nothing.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)

	return &Postgres{db: db}, nil
}

// Close releases the connection pool.
func (p *Postgres) Close() error { return p.db.Close() }

// DB exposes the pool, for EnsureSchema and for tests.
func (p *Postgres) DB() *sql.DB { return p.db }

// remoteSchema is the mirror.
//
// It is not the local schema. There is no FTS table — Postgres has its own
// full-text machinery and choosing a configuration for it is a decision
// nobody has asked for yet — and there is no deletions table, because this
// sync is one-directional: the local file is the authority and a tombstone
// has nobody else to inform.
//
// synced_at is the remote's own column, not a copy of anything local. It is
// what answers "when did this machine last reach us".
const remoteSchema = `
CREATE TABLE IF NOT EXISTS evomem_notes (
	id          TEXT PRIMARY KEY,
	project_id  TEXT NOT NULL,
	content     TEXT NOT NULL,
	source_type TEXT NOT NULL,
	created_at  TIMESTAMPTZ NOT NULL,
	updated_at  TIMESTAMPTZ NOT NULL,
	metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
	synced_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS evomem_notes_project_idx
	ON evomem_notes (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS evomem_notes_updated_idx
	ON evomem_notes (updated_at);
`

// EnsureSchema creates the mirror if it is not there. It is safe to run
// again: every statement is IF NOT EXISTS.
func (p *Postgres) EnsureSchema(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, remoteSchema); err != nil {
		return fmt.Errorf("sync: creating the remote schema: %w", err)
	}
	return nil
}

// Ping reports whether the remote can be reached and written to.
//
// Reached and written to, not merely dialled: a database that is up but has
// no table is not somewhere a batch can go, and finding that out here gives
// the user an error that says what to do instead of a push failure every
// five minutes.
func (p *Postgres) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := p.db.PingContext(ctx); err != nil {
		return fmt.Errorf("sync: reaching PostgreSQL: %w", err)
	}

	var exists bool
	if err := p.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		                WHERE table_name = 'evomem_notes')`).Scan(&exists); err != nil {
		return fmt.Errorf("sync: looking for the remote schema: %w", err)
	}
	if !exists {
		return errors.New("sync: the remote has no evomem_notes table; run evomem sync -init-remote")
	}
	return nil
}

// upsertNote is the one statement a push runs.
//
// The guard on the last line is what makes a retry safe in the other
// direction too. Batches are pushed oldest first, so an out-of-order delivery
// should not be able to put an older version of a note over a newer one; the
// condition drops such a write rather than applying it.
const upsertNote = `
INSERT INTO evomem_notes
	(id, project_id, content, source_type, created_at, updated_at, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
ON CONFLICT (id) DO UPDATE SET
	project_id  = EXCLUDED.project_id,
	content     = EXCLUDED.content,
	source_type = EXCLUDED.source_type,
	updated_at  = EXCLUDED.updated_at,
	metadata    = EXCLUDED.metadata,
	synced_at   = now()
WHERE evomem_notes.updated_at <= EXCLUDED.updated_at
`

// PushNotes upserts a batch in one transaction.
//
// One transaction, so a batch either arrives or does not. The worker moves
// its cursor on the strength of the batch succeeding, and a half-applied
// batch would leave the cursor claiming more than the remote holds.
func (p *Postgres) PushNotes(ctx context.Context, notes []*models.Note) error {
	if len(notes) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sync: starting a push: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertNote)
	if err != nil {
		return fmt.Errorf("sync: preparing the push: %w", err)
	}
	defer stmt.Close()

	for _, n := range notes {
		metadata, err := n.MarshalMetadata()
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			n.ID, n.ProjectID, n.Content, string(n.SourceType),
			n.CreatedAt.UTC(), n.UpdatedAt.UTC(), metadata); err != nil {
			return fmt.Errorf("sync: pushing note %s: %w", n.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sync: committing a push: %w", err)
	}
	return nil
}

// PushDeletions removes notes by id.
//
// An id that is not there is not an error: the local side may be reporting a
// note the remote never saw, which happens whenever a note is created and
// deleted between two passes.
func (p *Postgres) PushDeletions(ctx context.Context, deletions []database.Deletion) error {
	if len(deletions) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sync: starting a delete: %w", err)
	}
	defer tx.Rollback()

	// One statement per id rather than an array parameter. An array would
	// mean depending on the driver's own type mapping for text[], and a
	// prepared statement reused two hundred times is not the slow part of
	// a sync that only runs every few minutes.
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM evomem_notes WHERE id = $1`)
	if err != nil {
		return fmt.Errorf("sync: preparing the delete: %w", err)
	}
	defer stmt.Close()

	for _, d := range deletions {
		if _, err := stmt.ExecContext(ctx, d.ID); err != nil {
			return fmt.Errorf("sync: deleting note %s: %w", d.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sync: committing a delete: %w", err)
	}
	return nil
}

// Count is how many notes the remote holds, for a status line.
func (p *Postgres) Count(ctx context.Context) (int, error) {
	var n int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM evomem_notes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sync: counting remote notes: %w", err)
	}
	return n, nil
}

// PullNotes fetches notes from the remote that are newer than the cursor.
func (p *Postgres) PullNotes(ctx context.Context, cursorUpdatedAt, cursorID string, limit int) ([]*models.Note, string, string, error) {
	if limit <= 0 {
		limit = 1000
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_id, content, source_type, created_at, updated_at, metadata
		 FROM evomem_notes
		 WHERE updated_at > $1 OR (updated_at = $1 AND id > $2)
		 ORDER BY updated_at ASC, id ASC
		 LIMIT $3`,
		cursorUpdatedAt, cursorID, limit)
	if err != nil {
		return nil, "", "", fmt.Errorf("sync: pulling notes: %w", err)
	}
	defer rows.Close()

	var notes []*models.Note
	var nextUpdatedAt, nextID string
	for rows.Next() {
		var n models.Note
		var metadata string
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Content, &n.SourceType, &n.CreatedAt, &n.UpdatedAt, &metadata); err != nil {
			return nil, "", "", fmt.Errorf("sync: scanning note: %w", err)
		}
		if err := n.UnmarshalMetadata(metadata); err != nil {
			return nil, "", "", fmt.Errorf("sync: unmarshaling metadata: %w", err)
		}
		notes = append(notes, &n)
		nextUpdatedAt = n.UpdatedAt.Format(time.RFC3339Nano)
		nextID = n.ID
	}
	if err := rows.Err(); err != nil {
		return nil, "", "", fmt.Errorf("sync: iterating notes: %w", err)
	}
	return notes, nextUpdatedAt, nextID, nil
}

// PullAll fetches all notes from the remote (for restore).
func (p *Postgres) PullAll(ctx context.Context, limit int, offset int) ([]*models.Note, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_id, content, source_type, created_at, updated_at, metadata
		 FROM evomem_notes
		 ORDER BY updated_at ASC, id ASC
		 LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("sync: pulling all notes: %w", err)
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		var n models.Note
		var metadata string
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Content, &n.SourceType, &n.CreatedAt, &n.UpdatedAt, &metadata); err != nil {
			return nil, fmt.Errorf("sync: scanning note: %w", err)
		}
		if err := n.UnmarshalMetadata(metadata); err != nil {
			return nil, fmt.Errorf("sync: unmarshaling metadata: %w", err)
		}
		notes = append(notes, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync: iterating notes: %w", err)
	}
	return notes, nil
}
