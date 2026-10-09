package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// schemaVersion is the version this build writes and the only one it reads.
//
// Unlike a derived index, this file is the only copy of the user's notes: a
// version it does not recognise is an error the caller has to see, never a
// reason to start again.
const schemaVersion = 5

// The schema. Two things in it are worth explaining.
//
// First, notes declares rowid explicitly. An INTEGER PRIMARY KEY is an alias
// for the rowid and is therefore stable; an implicit rowid is not, because
// VACUUM is allowed to renumber it. The FTS index is joined to notes by rowid,
// so a renumbering would silently point every search result at the wrong note.
// id stays the identifier everything outside this package uses, and UNIQUE
// gives it the same guarantee PRIMARY KEY did.
//
// Second, notes_fts is an external content table: it indexes notes.content
// without storing a second copy of it. That saves roughly half the space on a
// store whose whole payload is text, at the cost of needing the triggers
// below, because SQLite does not keep an external content index in step on its
// own.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notes (
	rowid       INTEGER PRIMARY KEY,
	id          TEXT NOT NULL UNIQUE,
	project_id  TEXT NOT NULL,
	content     TEXT NOT NULL,
	source_type TEXT NOT NULL,
	created_at  DATETIME NOT NULL,
	updated_at  DATETIME NOT NULL,
	metadata    TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_notes_project ON notes(project_id);
CREATE INDEX IF NOT EXISTS idx_notes_project_created ON notes(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notes_source ON notes(source_type);

-- The order the sync engine walks: everything after a (updated_at, id)
-- watermark, in that order. Without this index that walk is a table scan on
-- every pass.
CREATE INDEX IF NOT EXISTS idx_notes_updated ON notes(updated_at, id);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
	content,
	content='notes',
	content_rowid='rowid',
	tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS notes_fts_insert AFTER INSERT ON notes BEGIN
	INSERT INTO notes_fts(rowid, content) VALUES (new.rowid, new.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_fts_delete AFTER DELETE ON notes BEGIN
	INSERT INTO notes_fts(notes_fts, rowid, content) VALUES ('delete', old.rowid, old.content);
END;

CREATE TRIGGER IF NOT EXISTS notes_fts_update AFTER UPDATE ON notes BEGIN
	INSERT INTO notes_fts(notes_fts, rowid, content) VALUES ('delete', old.rowid, old.content);
	INSERT INTO notes_fts(rowid, content) VALUES (new.rowid, new.content);
END;

-- Schema 2: what the sync engine needs.

-- A note the user deleted. The row is gone, so without a tombstone there is
-- nothing left to tell the far end it went: a delete would look exactly like
-- a note that was never there, and the cloud copy would live forever.
--
-- Written explicitly by Delete, never by a trigger. Archiving also removes
-- rows, and those must not be tombstoned: they have already been synced and
-- are being thinned locally, not deleted.
CREATE TABLE IF NOT EXISTS deletions (
	id         TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	deleted_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_deletions_at ON deletions(deleted_at, id);

-- How far the sync engine has got. One row per key, so a new kind of
-- watermark does not need a migration.
CREATE TABLE IF NOT EXISTS sync_state (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

-- Schema 3: what an agent proposed, which is not yet memory.
--
-- A separate table, not a column on notes. A proposal that lived in notes
-- with a flag would be one forgotten WHERE clause away from being searched,
-- synced and read back to a model as though a person had accepted it. Here
-- the only way into notes is Accept.
CREATE TABLE IF NOT EXISTS proposals (
	id          TEXT PRIMARY KEY,
	project_id  TEXT NOT NULL,
	content     TEXT NOT NULL,
	source_type TEXT NOT NULL,
	metadata    TEXT NOT NULL DEFAULT '{}',
	reason      TEXT NOT NULL DEFAULT '',
	proposed_by TEXT NOT NULL DEFAULT '',
	proposed_at DATETIME NOT NULL,

	-- pending until a person decides. A decided row is kept so that the
	-- same thing is not proposed again the next day, and so there is a
	-- record of what was turned down; archiving clears the old ones.
	status     TEXT NOT NULL DEFAULT 'pending',
	decided_at DATETIME,
	note_id    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_proposals_pending
	ON proposals(status, proposed_at DESC);

-- A grouping an agent made over notes that are already there (ADR-0023).
-- Not a field on a note: a note belongs to as many clusters as make sense,
-- and a rename here touches one row rather than every member.
CREATE TABLE IF NOT EXISTS clusters (
	id         TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	name       TEXT NOT NULL,
	summary    TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_clusters_project ON clusters(project_id, updated_at DESC);

-- Membership, cascading both ways: deleting a note takes its memberships
-- with it, and a cluster cannot name a note that is gone.
CREATE TABLE IF NOT EXISTS cluster_notes (
	cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
	note_id    TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	added_at   DATETIME NOT NULL,
	PRIMARY KEY (cluster_id, note_id)
);

CREATE INDEX IF NOT EXISTS idx_cluster_notes_note ON cluster_notes(note_id);

-- A source somebody else owns, and what it takes to call it (ADR-0025).
-- secret holds ciphertext sealed with a key from the environment, never the
-- token: a stolen copy of this file is not enough on its own.
CREATE TABLE IF NOT EXISTS connections (
	id             TEXT PRIMARY KEY,
	source_type    TEXT NOT NULL,
	project_id     TEXT NOT NULL,
	base_url       TEXT NOT NULL,
	account        TEXT NOT NULL DEFAULT '',
	secret         BLOB NOT NULL,
	query          TEXT NOT NULL DEFAULT '',
	cursor         TEXT NOT NULL DEFAULT '',
	last_pulled_at DATETIME,
	last_error     TEXT NOT NULL DEFAULT '',
	created_at     DATETIME NOT NULL,
	updated_at     DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_connections_source ON connections(source_type, project_id);
`

// migrate creates the schema if it is absent, brings an older file forward,
// and refuses a file written by a version this build does not know.
func (d *DB) migrate(ctx context.Context) error {
	// Every object is CREATE ... IF NOT EXISTS, so this both creates a
	// fresh store and adds whatever a later version introduced. A step
	// that has to touch existing rows goes in migrations below instead.
	if err := d.createSchema(ctx); err != nil {
		return err
	}

	var version int
	err := d.write.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	switch {
	case err == sql.ErrNoRows:
		// The column is TEXT and the value goes in as text. SQLite
		// would coerce an int; PostgreSQL refuses to guess, and
		// being explicit is right for both.
		_, err := d.write.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES('schema_version', ?)`,
			strconv.Itoa(schemaVersion))
		if err != nil {
			return fmt.Errorf("database: recording the schema version: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("database: reading the schema version: %w", err)
	case version == schemaVersion:
		return nil
	case version > schemaVersion:
		// Forward only. An older build must not touch a file a newer
		// one has written: it would not know what the new objects mean
		// and this file is the only copy of the user's notes.
		return fmt.Errorf("database: %s was written by a newer Evomem (schema %d, this build reads %d)",
			d.path, version, schemaVersion)
	}

	return d.upgrade(ctx, version)
}

// createSchema runs the declarative schema for whichever SQL this store
// speaks (ADR-0028).
//
// SQLite takes the whole thing in one call; pgx sends one statement per
// message, so PostgreSQL's is a list and each statement goes on its own.
func (d *DB) createSchema(ctx context.Context) error {
	if d.write.dialect == dialectPostgres {
		for _, stmt := range postgresSchema {
			if _, err := d.write.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("database: creating the schema: %w\n%s", err, stmt)
			}
		}
		return nil
	}
	if _, err := d.write.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("database: creating the schema: %w", err)
	}
	return nil
}

// migrations holds the steps that have to touch existing rows. Index i brings
// a file from version i+1 to version i+2; a nil entry means the declarative
// schema above was the whole change.
//
// Schemas 2 and 3 each added tables and indexes and nothing else, so there is
// nothing to do here for either. The slots exist so that the next version has
// an obvious place, and so that the version number and the steps cannot drift
// apart silently.
var migrations = []func(context.Context, *txn) error{
	nil,            // 1 -> 2
	nil,            // 2 -> 3
	addClusters,    // 3 -> 4
	addConnections, // 4 -> 5
}

// addConnections creates the table ADR-0025 introduced.
func addConnections(ctx context.Context, tx *txn) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS connections (
			id             TEXT PRIMARY KEY,
			source_type    TEXT NOT NULL,
			project_id     TEXT NOT NULL,
			base_url       TEXT NOT NULL,
			account        TEXT NOT NULL DEFAULT '',
			secret         BLOB NOT NULL,
			query          TEXT NOT NULL DEFAULT '',
			cursor         TEXT NOT NULL DEFAULT '',
			last_pulled_at DATETIME,
			last_error     TEXT NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL,
			updated_at     DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_source ON connections(source_type, project_id)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// addClusters creates the tables ADR-0023 introduced. The statements are the
// ones in schemaSQL, so a store created at 4 and one upgraded to it are the
// same store.
func addClusters(ctx context.Context, tx *txn) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS clusters (
			id         TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			name       TEXT NOT NULL,
			summary    TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_clusters_project ON clusters(project_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS cluster_notes (
			cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
			note_id    TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			added_at   DATETIME NOT NULL,
			PRIMARY KEY (cluster_id, note_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_notes_note ON cluster_notes(note_id)`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// upgrade runs every migration from the file's version up to this build's,
// one transaction each, recording the version as it goes. A step that fails
// leaves the file at the last version that succeeded.
func (d *DB) upgrade(ctx context.Context, from int) error {
	if want := schemaVersion - 1; len(migrations) != want {
		// A version was bumped without a slot, or the other way round.
		return fmt.Errorf("database: %d migration steps for schema version %d", len(migrations), schemaVersion)
	}

	for version := from; version < schemaVersion; version++ {
		step := migrations[version-1]
		tx, err := d.write.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("database: upgrading to schema %d: %w", version+1, err)
		}
		if step != nil {
			if err := step(ctx, tx); err != nil {
				tx.Rollback()
				return fmt.Errorf("database: upgrading to schema %d: %w", version+1, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE meta SET value = ? WHERE key = 'schema_version'`, strconv.Itoa(version+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("database: recording schema %d: %w", version+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("database: upgrading to schema %d: %w", version+1, err)
		}
	}
	return nil
}

// SchemaVersion returns the version recorded in the file.
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := d.read.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil {
		return 0, fmt.Errorf("database: reading the schema version: %w", err)
	}
	return version, nil
}

// RebuildIndex rebuilds the full-text index from notes. The index is derived,
// so this is always safe; it is what to run if a search ever disagrees with
// the table.
func (d *DB) RebuildIndex(ctx context.Context) error {
	_, err := d.write.ExecContext(ctx, `INSERT INTO notes_fts(notes_fts) VALUES('rebuild')`)
	if err != nil {
		return fmt.Errorf("database: rebuilding the index: %w", err)
	}
	return nil
}
