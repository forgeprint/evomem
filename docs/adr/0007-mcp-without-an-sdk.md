# ADR-0007: The MCP server is written against the specification, and speaks both eras

**Status:** accepted · **Date:** 2026-10-06

## Context

`plan.md` says to "wire up the official or compliant lightweight Go MCP server
framework". Every such framework is a new dependency, which ADR-0002 rules out
without a decision of its own. The protocol over stdio is newline-delimited
JSON-RPC 2.0 with four methods this server needs.

The harder question is which revision. Verified against the specification on
2026-10-06:

- `2026-07-28` is current. It removed the `initialize` handshake and the
  protocol-level session: every request carries
  `io.modelcontextprotocol/protocolVersion` and
  `io.modelcontextprotocol/clientCapabilities` in `_meta`, and
  `server/discover` is mandatory.
- `2025-11-25` and earlier open with `initialize` and keep the negotiated
  version for the connection.

The specification's own compatibility matrix is explicit about the cost of
picking one: a modern-only server fails a legacy client, and a legacy client
has no fall-forward mechanism, so the failure is terminal rather than a
downgrade.

## Decision

Write the server by hand in `core/mcp`, against the specification, with no SDK.
Implement both eras: a request carrying modern `_meta` is served statelessly,
an `initialize` request selects legacy semantics for the process.

## Consequences

- No dependency, and the protocol is readable in about 200 lines.
- Both client eras work. `TestLegacyHandshake` and the modern `_meta` tests
  are what hold each half up.
- One piece of connection state is kept — that `initialize` was seen — and
  only the legacy era may use it. Every modern request is checked on its own,
  because the modern protocol forbids inferring anything from a previous one.
- A revision after `2026-07-28` is work here rather than a version bump in
  `go.mod`. `Supported` and the two constants are the whole surface of that.
- Protocol details are never written from memory. The constants, the `-32022`
  error shape, the `resultType` requirement and the `cacheScope`/`ttlMs` pair
  were each checked against the published specification, and the package
  comment cites the page and the date.
- stdout belongs to the protocol alone. `evomem mcp` writes everything else to
  stderr; a stray print on stdout corrupts the stream for the client.
