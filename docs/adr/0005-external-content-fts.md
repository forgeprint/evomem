# ADR-0005: An external content FTS index, and an explicit rowid

**Status:** accepted · **Date:** 2026-10-06

## Context

`plan.md` specifies `notes` with `id TEXT PRIMARY KEY` and
`notes_fts USING fts5(id, content, content='notes')`. A note's whole payload is
text, so an ordinary FTS5 table would store every note twice.

Two problems with the specified form:

1. With `content='notes'`, FTS5 finds a row by rowid. `notes` as specified has
   no `INTEGER PRIMARY KEY`, so its rowid is implicit — and `VACUUM` is allowed
   to renumber an implicit rowid. Phase 5's archiving will want to vacuum. A
   renumbering would point every search result at the wrong note, silently.
2. Indexing `id` as full text costs space and finds nothing: a ULID is not
   something anyone searches for by word.

## Decision

Keep the external content table, and change two things:

- `notes` declares `rowid INTEGER PRIMARY KEY` explicitly, which makes it a
  rowid alias and therefore stable across `VACUUM`. `id TEXT NOT NULL UNIQUE`
  keeps the same guarantee `PRIMARY KEY` gave it, and `id` remains the only
  identifier anything outside `shared/database` sees.
- `notes_fts` indexes `content` alone, keyed on
  `content='notes', content_rowid='rowid'`.

The atomic pillars named in `CLAUDE.md` — `id`, `project_id`, `content`,
`source_type`, `created_at`, `updated_at` — are unchanged.

## Consequences

- Roughly half the space of a duplicating index.
- SQLite does not keep an external content index in step on its own, so three
  triggers do: insert, delete, and update as a delete followed by an insert.
  A write that bypasses them leaves the index stale.
  `TestSearchSeesANewNote`, `TestSearchFollowsAnUpdate` and
  `TestSearchForgetsADeletedNote` are what prove they are wired.
- The index is derived, so `DB.RebuildIndex` can always discard and rebuild it.
- A future `metadata` search needs its own decision: adding a column to
  `notes_fts` means rebuilding the index.
