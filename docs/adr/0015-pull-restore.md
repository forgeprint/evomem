# ADR-0015: Pull/Restore from mirror

**Status:** accepted · **Date:** 2026-10-08 · **Amended by:** ADR-0032, ADR-0033

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

## Accepted, and where the build went past this record

Marked accepted on 2026-10-12, late: `evomem pull` and
`evomem restore -confirm` have both been shipped and used for days while
this said "proposed". A record left proposed after the thing is running
stops being a decision and becomes a guess about the code.

Three details differ from what is written above, each on purpose and each
recorded where it was decided:

- **The flag is `-confirm`, not `--confirm`.** Go's `flag` package, which
  this project uses rather than a CLI framework; both spellings are
  accepted by it and the help text says one.

- **Pull has its own watermark, not "the same `sync_state` cursor".**
  Sharing one cursor between the two directions made a push move the pull's
  position, so a pull could skip notes it had never seen. → ADR-0032

- **Pull upserts by "absorb", not plain `ON CONFLICT`.** A blind upsert let
  an out-of-date mirror overwrite a newer local note, so an absorb skips a
  row that is not newer than what is here. Deletions arrive as tombstones
  rather than being inferred from absence. → ADR-0032, ADR-0033

"Local is authority; pull only adds missing" survives as the intent, with
"newer wins per note" as the rule that implements it.
