# The Evomem MCP server

`evomem mcp` speaks the Model Context Protocol on stdin and stdout. A coding
agent starts it as a subprocess; it is not something to run by hand, except to
test it (see below).

It implements both protocol eras: `2026-07-28` (stateless, per-request `_meta`,
`server/discover`) and `2025-11-25` (the `initialize` handshake). See
[ADR-0007](adr/0007-mcp-without-an-sdk.md) for why both.

## Tools

Four read, one proposes. Nothing writes to memory
([ADR-0008](adr/0008-read-only-tools.md),
[ADR-0013](adr/0013-proposals-need-a-person.md)).

| Tool | Arguments | Returns |
| - | - | - |
| `search_notes` | `query`, and optionally `project_id`, `source_type`, `prefix`, `limit` | matching notes, newest-ranked first, with the matching stretch marked |
| `get_note` | `id` | one note in full, uncut |
| `get_project_context` | `project_id`, and optionally `source_type`, `limit`, `offset` | the project's notes, newest first, with `total` and the next `offset` |
| `list_projects` | none | every project with notes, a count, and when the newest was written |
| `propose_note` | `project_id`, `content`, and optionally `reason`, `tainted` | a proposal id, and `"remembered": false` |

`query` is not a query language: operators and punctuation are stripped and
every word has to appear. All results are bounded, and a note cut at 2000
characters is flagged `truncated` with a pointer to `get_note`.

## Configuring a client

The server needs the binary and, optionally, `EVOMEM_DB`. Without it the store
is `~/.evomem/evomem.db`.

### Claude Code

```sh
claude mcp add evomem -- /absolute/path/to/evomem mcp
```

### A client configured by JSON

```json
{
  "mcpServers": {
    "evomem": {
      "command": "/absolute/path/to/evomem",
      "args": ["mcp"],
      "env": { "EVOMEM_DB": "/absolute/path/to/evomem.db" }
    }
  }
}
```

An absolute path, because the client's working directory is its own.

## Testing it from a terminal

The protocol is newline-delimited JSON, so a pipe is enough. Every modern
request must carry its version and the client's capabilities:

```sh
M='"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}'

printf '%s\n' \
  "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"server/discover\",\"params\":{$M}}" \
  "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{$M}}" \
  "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{$M,\"name\":\"list_projects\",\"arguments\":{}}}" \
  | evomem mcp -quiet
```

A legacy client opens with the handshake and sends no `_meta` after it:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_projects","arguments":{}}}' \
  | evomem mcp -quiet
```

`-quiet` suppresses the one line on stderr that names the store. stdout carries
nothing but protocol messages either way.

## Proposing

`propose_note` is the only tool that writes, and what it writes is not memory.
A proposal goes into its own table: it cannot be searched, cannot be read back
by `get_note`, does not appear in `get_project_context`, and is never pushed to
the PostgreSQL mirror. The only path into `notes` is a person running:

```sh
evomem review                    # what is waiting
evomem review -accept <id>       # store it as a note
evomem review -reject <id>       # turn it down
```

The queue holds at most 200 pending proposals; going over is a tool error that
tells the agent to stop rather than to rephrase. An identical proposal that is
still pending returns the one already waiting rather than a second.

`tainted` is the agent's own declaration that the content came from outside the
project — a web page, a chat message, someone else's document. It is
self-declared because the agent is the only party that knows, and
`evomem review` prints it where the person deciding will read it.

An accepted note is **not** marked tainted even if the proposal was: a person
read it and said yes. The claim is kept as `proposed_tainted` in the note's
metadata, next to `proposed_by` and `proposal_id`.

## What it does not do

No resources, no prompts, no sampling: `capabilities` says `tools` and nothing
else, and `listChanged` is `false` because the set is fixed at build time.

No tool writes to memory, and there is no accept-with-edit: accept a proposal
as it stands, or reject it and write your own with `evomem add`. See
[ADR-0013](adr/0013-proposals-need-a-person.md).
