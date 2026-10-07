# ADR-0014: Telegram multi-project routing

**Status:** proposed · **Date:** 2026-10-08

## Context

Telegram adapter currently routes all messages to a single project (`EVOMEM_TELEGRAM_PROJECT`). Users want messages from different chats/groups to go to different projects.

Three routing strategies:

1. **chat_id → project mapping** — Configure a map `chat_id: project_id` in settings
2. **#tag in message** — User includes `#projectname` in message, parsed at ingest
3. **Bot command** — `/project <name>` sets context for subsequent messages

Only one can be active; they conflict (which takes precedence?).

## Decision

**Option 1: chat_id → project mapping** as the primary mechanism.

- Deterministic, no message parsing needed
- Works for channels/groups where bot can't read all messages
- Configurable via `telegram_chat_projects` JSON in settings (or env var)
- Falls back to `EVOMEM_TELEGRAM_PROJECT` if no match

Hashtag parsing added as secondary (opt-in): if message starts with `#projectname `, use that project instead.

Bot command `/project` not implemented — adds complexity for marginal benefit.

## Consequences

- Settings UI needs chat_id ↔ project mapping editor
- `telegram_chat_id` already stored in metadata, no migration needed
- Backwards compatible: empty mapping = all to default project