# ADR-0014: Telegram multi-project routing

**Status:** accepted · **Date:** 2026-10-08

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

## Accepted, and the one thing that moved

Marked accepted on 2026-10-12, late: this shipped as decided and sat here
as "proposed" while running. Both halves are in
`core/api/adapters/telegram/telegram.go` — the chat map first, a leading
`#projectname` second, `EVOMEM_TELEGRAM_PROJECT` last — and `/project` was
not implemented, as decided.

The configuration is **`EVOMEM_TELEGRAM_CHAT_PROJECTS`, an environment
variable**, not a settings row or a mapping editor. This record left the
choice open ("in settings (or env var)"); the env var is what ADR-0010
requires of anything that configures an ingestion path, and the panel grew
connections rather than a routing editor (ADR-0026).

So "Settings UI needs chat_id ↔ project mapping editor", under
Consequences above, did not happen and is not planned. Routing is changed
by changing the server's environment, which is where the secret beside it
already lives.
