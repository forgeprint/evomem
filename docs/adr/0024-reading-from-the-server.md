# ADR-0024: Authority is split by kind, and the server gains a read side

**Status:** accepted · **Date:** 2026-10-09 · Amends ADR-0012 and ADR-0023.

## Context

The purpose has been stated plainly: every note — typed, or arriving from
Jira, Apple Notes, Telegram — is the user's **memory**, offered to Claude and
other AI platforms over MCP. Notes taken on the phone sync to a server, the
server organises them into clusters, and **the phone sees and edits those
clusters too**.

Two things stood in the way, and both are recorded decisions.

**ADR-0012 made sync one-way** — local SQLite is the authority, the remote is
a mirror — because two-way merge "would need per-field conflict resolution and
a clock nobody trusts". Editing a cluster on the phone means data travelling
server → client, which nothing does today.

**The HTTP server has no read side at all.** Every route is a write
(`/ingest`, `/ingest/audio`, `PUT` and `DELETE /notes/{id}`) or a webhook;
`/healthz` says only that the process is up. A client cannot ask the server
anything.

## Decision

### 0. A source is either mine or somebody else's

The phone is **a source**, the way Jira and Telegram are — not a peer holding
a second copy of the memory. That framing decides the direction of every
sync:

- **A source I own pushes.** The phone sends what it holds to the platform
  when a person asks it to. Built: `/ingest`, `PUT` and `DELETE` under a
  bearer token.
- **A source somebody else owns is pulled.** Asking the platform to sync Jira
  or Apple Notes means the platform calling *their* API with credentials the
  user gave it.

The second direction does not exist. Jira and Telegram arrive here by
**webhook** today — they push to us, on their schedule, if somebody
configured them to. A connector the hub calls out to, on demand, needs
per-source credentials, a per-source cursor and something to trigger it, and
is its own decision.

What this record changes now is that the phone finally says which source it
is: it filed notes as `manual`, indistinguishable from one typed into the
command line, and `GET /notes?source=mobile` would have answered with
nothing.

### 1. Authority is split by kind, not by device

- **A note is authored.** It belongs to the device it was written on, which
  keeps it and pushes it. ADR-0012 stands: one way, no merge, offline-capable.
- **A cluster is derived.** It is an opinion about notes that already exist,
  it is made on the server by something with a view of all of them, and it
  lives **only** there. Clients read it and edit it over HTTP.

This is what avoids two-way sync without giving up the feature. A cluster is
never produced in two places at once, so there is nothing to merge: the thing
that made conflict resolution necessary does not arise.

**What it costs:** clusters are not available offline. A phone with no signal
shows the notes it holds and no groupings at all, and an edit to a grouping
fails rather than queueing. Notes keep working offline, which is the half
that matters for capture.

### 2. A read side, under the same posture as the write side

```
GET /notes?project=&source=&q=&limit=&offset=
GET /clusters?project=
GET /clusters/{id}
```

Registered only with `EVOMEM_API_TOKEN`, bearer-authenticated, no token no
endpoint (ADR-0010). Limits are clamped by the store, as everywhere else.

`GET /notes` is **a view, not a sync mechanism**. A client that wants to see
what the server holds can ask; a client's own notes are still its own, and
nothing here pulls them down into another store. Calling this a pull would
reopen exactly the question ADR-0012 closed.

Unlike the MCP tools, these answers carry no warning lines. The marks are in
the metadata — `tainted`, `transcribed`, `endorsed_at` — and a client renders
them for a person. The lines exist because a model reads prose; a client does
not need to be told twice.

### 3. CORS is configured, never assumed

A browser calling this from another origin is blocked without it, and the
browser is where this is meant to be tried first. `EVOMEM_CORS_ORIGIN` names
the origins allowed, comma-separated. **Unset means no origin is allowed** —
the same shape as every secret here: absent is off, not open.

No wildcard is accepted. An endpoint that serves a bearer token's worth of
somebody's memory to `*` is an endpoint that any page they visit can read.

## Consequences

### Costs accepted

- **The server becomes readable.** One leaked token used to mean somebody
  could write notes into the store; it now means they can read all of it. For
  a store whose purpose is to be somebody's memory, that is the more serious
  of the two, and the mitigation is still ADR-0010's: loopback by default and
  a tunnel the user raises deliberately.
- **Clusters are online-only**, named above.
- **A second way to read a note.** MCP and HTTP now both serve them, with
  different framing, and a change to what a note exposes has to land in both.
- **CORS is a list someone has to maintain**, and getting it wrong is either
  a browser that cannot call the server or an origin that should not have
  been trusted.

### Explicitly not decided here

- **Editing clusters over HTTP.** This record decides that clients may, and
  the read side is what the next piece builds on. The write routes are their
  own step.
- **Serving MCP over HTTP.** The binary speaks MCP on stdio, so Claude talks
  to whichever copy runs beside it. Reaching a store on a VPS from a laptop
  needs MCP over a transport that crosses machines, which is the centre of the
  stated purpose and is a decision of its own.
- **Pull connectors for third-party sources.** Named above: the hub calling
  Jira's or Apple Notes' API rather than waiting for a webhook. It is the
  other half of what sync means and it needs a place to keep credentials,
  which is the same place the panel's API key goes.
- **Server-side clustering from a panel API key.** ADR-0023 chose the agent
  over MCP and rejected evomem calling a model service; the user wants that
  path back as an option, configured from a panel. It is an amendment to
  ADR-0023 and is not made here.
