package database

// The same tables as the SQLite schema, in the SQL PostgreSQL speaks
// (ADR-0028).
//
// A slice rather than one string because pgx sends statements over the
// extended protocol, which takes exactly one per message. Splitting a blob on
// semicolons would work until the first statement that contains one; a list
// says where each statement ends without guessing.
//
// Three deliberate differences from the SQLite schema, each with a reason:
//
//   - **No rowid.** ADR-0005 declares one in SQLite because the FTS index is
//     joined by it and VACUUM may renumber an implicit one. PostgreSQL has no
//     rowid and its search will be a column on this table, so `id` is simply
//     the primary key.
//
//   - **Timestamps are TEXT, not TIMESTAMPTZ.** The Go code formats RFC3339
//     and compares lexicographically — the sync watermark walks
//     (updated_at, id) as strings, and archiving compares against a formatted
//     cutoff. TIMESTAMPTZ would change what those comparisons mean and would
//     need every Scan rewritten. This migration is supposed to preserve
//     behaviour, so the column holds what the code already writes.
//     The cost is real and named: no date arithmetic in SQL, and a range
//     query is a string range. Worth revisiting once the behaviour is
//     equal, not during the move.
//
//   - **No full-text objects.** FTS5 is a virtual table and three triggers;
//     PostgreSQL wants a tsvector and a GIN index, and ADR-0003's recorded
//     behaviour has to be measured against it rather than assumed. That is
//     its own step and it is not this one.
var postgresSchema = []string{
	`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS notes (
		id          TEXT PRIMARY KEY,
		project_id  TEXT NOT NULL,
		content     TEXT NOT NULL,
		source_type TEXT NOT NULL,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL,
		metadata    TEXT NOT NULL DEFAULT '{}'
	)`,

	`CREATE INDEX IF NOT EXISTS idx_notes_project ON notes(project_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notes_project_created ON notes(project_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_notes_source ON notes(source_type)`,

	// The order the sync engine walks: everything after a
	// (updated_at, id) watermark, in that order.
	`CREATE INDEX IF NOT EXISTS idx_notes_updated ON notes(updated_at, id)`,

	`CREATE TABLE IF NOT EXISTS deletions (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		deleted_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_deletions_at ON deletions(deleted_at, id)`,

	`CREATE TABLE IF NOT EXISTS sync_state (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS proposals (
		id          TEXT PRIMARY KEY,
		project_id  TEXT NOT NULL,
		content     TEXT NOT NULL,
		source_type TEXT NOT NULL,
		metadata    TEXT NOT NULL DEFAULT '{}',
		reason      TEXT NOT NULL DEFAULT '',
		proposed_by TEXT NOT NULL DEFAULT '',
		proposed_at TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'pending',
		decided_at  TEXT,
		note_id     TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_proposals_pending
		ON proposals(status, proposed_at DESC)`,

	`CREATE TABLE IF NOT EXISTS clusters (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		name       TEXT NOT NULL,
		summary    TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_clusters_project ON clusters(project_id, updated_at DESC)`,

	`CREATE TABLE IF NOT EXISTS cluster_notes (
		cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
		note_id    TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
		added_at   TEXT NOT NULL,
		PRIMARY KEY (cluster_id, note_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_cluster_notes_note ON cluster_notes(note_id)`,

	// secret holds ciphertext sealed with a key from the environment,
	// never the token (ADR-0025). BYTEA where SQLite has BLOB.
	`CREATE TABLE IF NOT EXISTS connections (
		id             TEXT PRIMARY KEY,
		source_type    TEXT NOT NULL,
		project_id     TEXT NOT NULL,
		base_url       TEXT NOT NULL,
		account        TEXT NOT NULL DEFAULT '',
		secret         BYTEA NOT NULL,
		query          TEXT NOT NULL DEFAULT '',
		cursor         TEXT NOT NULL DEFAULT '',
		last_pulled_at TEXT,
		last_error     TEXT NOT NULL DEFAULT '',
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_connections_source ON connections(source_type, project_id)`,
}
