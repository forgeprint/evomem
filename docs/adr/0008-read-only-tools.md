# ADR-0008: The MCP tool set is read-only, for now

**Status:** accepted, amended by [ADR-0013](0013-proposals-need-a-person.md) · **Date:** 2026-10-06

## Context

`plan.md` Phase 2 asks for two tools: `search_notes` and
`get_project_context`. Neither writes. The obvious third — letting an agent
record a note — was not asked for, and is the one with consequences.

## Decision

Four read-only tools:

| Tool | What it is for |
| - | - |
| `search_notes` | full-text search across every note |
| `get_note` | one note in full, by id |
| `get_project_context` | a project's notes, newest first |
| `list_projects` | what projects exist |

`get_note` and `list_projects` are additions to the plan, and each closes a
hole in the two that were specified. A search result cuts a long note at
`maxContentChars` so one transcribed recording cannot crowd out the other
nineteen hits, which leaves no way to read the rest — that is `get_note`.
`get_project_context` requires a `project_id`, and nothing else in the tool set
said what projects exist — that is `list_projects`.

Writing stays out. It is `evomem add` and, from Phase 4, the HTTP entrypoint.

> **Amended 2026-10-06.** ADR-0013 adds a fifth tool, `propose_note`. The tool
> set is no longer read-only, but memory still is: a proposal goes into its own
> table and only a person accepting it with `evomem review` creates a note.
> Everything below about bounded results and tool errors still holds.

## Consequences

- An agent cannot put anything into the user's memory, so nothing it writes
  can be wrong in a store the user trusts. What a write tool should look like —
  a proposal a person accepts, or a direct write — is a decision Phase 4 has to
  make alongside the ingestion adapters, and it needs its own ADR.
- Every result is bounded, because a tool result goes straight into a model's
  context: `defaultSearchLimit` 20, `defaultContextLimit` 25,
  `maxContentChars` 2000 per note. A cut note carries `truncated: true` and
  the text points at `get_note`, so a model does not answer from half a note.
- `get_project_context` reports `total` against `returned` and names the next
  `offset`, so a model reading the newest page can tell it is not everything.
- A missing or malformed argument is a tool error in the result (`isError`),
  not a JSON-RPC error: the specification wants it that way so the model can
  self-correct, and these are all things it can fix by calling again. An
  unknown tool is a protocol error, because trying different arguments cannot
  fix it.
