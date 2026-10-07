# ADR-0015: Pull/Restore from mirror

**Status:** proposed · **Date:** 2026-10-08

## Context

Sync is currently one-way: local SQLite → remote PostgreSQL (mirror).
Users need to pull data to a new device or restore after data loss.

The mirror schema (`evomem_notes`) is simple enough to support bidirectional sync:
- `notes` table with `updated_at` cursor
- `deletions` tombstone table
- No `synced` column on notes (cursor handles it)

## Decision

Implement **pull + restore** as separate commands, not automatic bidirectional sync.

### Pull (`evomem pull`)
- Fetches notes newer than local cursor from mirror
- Upserts by id (ON CONFLICT on id)
- Updates local cursor after successful batch
- Safe to run multiple times (idempotent)

### Restore (`evomem restore`)
- Full overwrite: deletes all local notes, pulls all from mirror
- Requires explicit `--confirm` flag
- Use case: new device setup, corrupted local DB

### Not implementing
- Automatic bidirectional sync (conflicts with "local-first" philosophy)
- Conflict resolution UI (local is authority; pull only adds missing)

## Consequences

- New commands: `evomem pull`, `evomem restore --confirm`
- Mirror schema unchanged (already supports)
- Pull uses same `sync_state` cursor mechanism
- Restore truncates local tables, re-initializes cursor