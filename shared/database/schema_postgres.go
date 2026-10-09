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
//   - **Full text is a configuration and a GIN index, not a virtual table.**
//     FTS5 is a virtual table and three triggers kept in step by hand;
//     PostgreSQL indexes an expression over `notes.content`, so there is
//     nothing to keep in step and nothing that can go stale. What the
//     configuration does, and where it differs from FTS5, is measured and
//     recorded beside it below.
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

// postgresSearchSchema builds the full-text objects.
//
// Separate from the tables because it is the one part that needs a privilege
// an ordinary user may not have — CREATE EXTENSION — and a store that cannot
// create it should say so about search rather than fail to open at all.
//
// `unaccent` ships with the official PostgreSQL image: `pg_available_extensions`
// lists it at 1.1 on postgres:17-alpine (checked 2026-10-09). It is contrib,
// not a Turkish dictionary, so the image stays stock — the thing ADR-0028
// said to avoid.
//
// The configuration exists because the obvious spelling does not work, and
// that was measured rather than assumed: both `unaccent(text)` and
// `unaccent(regdictionary, text)` are declared STABLE, so neither can appear
// in an index expression — PostgreSQL refuses with "functions in index
// expression must be marked IMMUTABLE". A text search configuration can,
// because `to_tsvector(regconfig, text)` is immutable, so the folding moves
// into a dictionary mapping instead of a function call.
//
// COPY = simple means no stemming, which is what FTS5 does too: ADR-0003
// records that `kilitlendi` does not find `kilitlenmek` and that stays true
// here. PostgreSQL has no built-in Turkish dictionary, so `simple` is also
// the only honest choice.
var postgresSearchSchema = []string{
	// In public so one extension serves every schema, rather than one per
	// schema in a database that holds several.
	`CREATE EXTENSION IF NOT EXISTS unaccent WITH SCHEMA public`,

	// CREATE TEXT SEARCH CONFIGURATION has no IF NOT EXISTS, so opening a
	// store that already has one has to be a no-op rather than an error.
	//
	// Both exceptions, because the one it actually raises is not the
	// obvious one: a configuration whose name is taken comes back as
	// unique_violation on pg_ts_config_cfgname_index, not duplicate_object.
	// Measured, after catching only duplicate_object failed on the second
	// open.
	`DO $$ BEGIN
		CREATE TEXT SEARCH CONFIGURATION evomem (COPY = simple);
	EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
	END $$`,

	// Unqualified, so it lands in and resolves from whatever schema the
	// search_path puts first. The index below and every query that uses
	// it resolve the same way, so they cannot disagree.
	`ALTER TEXT SEARCH CONFIGURATION evomem
		ALTER MAPPING FOR hword, hword_part, word WITH unaccent, simple`,

	`CREATE INDEX IF NOT EXISTS idx_notes_search
		ON notes USING GIN (to_tsvector('evomem', content))`,
}
