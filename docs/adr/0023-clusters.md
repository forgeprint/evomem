# ADR-0023: An agent groups notes into clusters, and the grouping is not a proposal

**Status:** accepted · **Date:** 2026-10-09

## Context

The direction is that notes should settle into groups a model has made sense
of, and that the browser is where that gets tried first. Nothing in the store
expresses a grouping today: a note has a project and a source, and that is
all the structure there is.

Three things were already decided and they shape this one more than any new
reasoning does:

- **Evomem hosts no model.** ADR-0002 allows the standard library and one
  dependency; ADR-0016 kept transcription outside the binary for the same
  reason.
- **The MCP tools are read-only** (ADR-0008), and the single tool that writes,
  `propose_note`, writes a proposal rather than a note.
- **A proposal needs a person** (ADR-0013): nothing an agent suggests is in
  memory until somebody accepts it.

## Decision

### 1. The agent does the grouping, over MCP

Claude reads notes with the tools that already exist and writes groupings with
new ones. Evomem gains no model, no inference dependency and no second wire
format; what it gains is five tools and two tables.

The alternative shapes were considered and rejected:

- **Evomem calling a model service**, the way `evomem transcribe` calls one.
  It would organise on a timer with nobody present, and it would put the cost,
  the key and the privacy of every note in the catalog into an operator's
  hands. Transcription earns that because audio is unreadable without it; a
  note is already readable.
- **Embeddings and clustering in Go.** A large dependency against ADR-0002,
  and a result nobody can argue with when it is wrong.

### 2. Clusters are their own tables, not a field on a note

```
clusters(id, project_id, name, summary, created_at, updated_at)
cluster_notes(cluster_id, note_id, added_at)
```

A note belongs to as many clusters as make sense, a cluster can be renamed
without rewriting its members, and both can be queried. `metadata` is the
extension point for *a source's own attributes* (CLAUDE.md), and a grouping
belongs to neither the note nor the source: it is a relationship between
notes, and a relationship in a JSON column means one cluster per note and a
rename that touches every row.

`cluster_notes` cascades on both sides, so deleting a note takes its
memberships with it and a cluster cannot name a note that is gone.

### 3. A grouping is applied, not proposed

This is the first break from ADR-0013, and it is deliberate.

That record exists because a note is **content entering memory**: once it is
there a model reads it as something the user said, so a person decides. A
cluster is not content. It is a label over notes that are already there, it
changes nothing a note says, and it is undone by deleting it. Organising five
hundred notes behind a review queue would mean five hundred decisions, and a
feature nobody uses protects nothing.

**What this costs:** an agent can now change what the store looks like
without anybody saying yes. It cannot change what any note *says* — the note
tools are still read-only and `propose_note` still queues — but a person
opening the app can find their notes arranged by something they did not ask
for. `evomem clusters` is how they see what happened, and deleting a cluster
is how they undo it.

### 4. Five tools, and a person can see the result without an agent

| tool | what it does |
| - | - |
| `create_cluster` | names a cluster and puts notes in it |
| `update_cluster` | renames, re-summarises, adds and removes members |
| `delete_cluster` | removes the grouping, never the notes |
| `list_clusters` | what exists in a project, with sizes |
| `get_cluster` | one cluster and the notes in it |

`evomem clusters` lists them from the command line, because a grouping an
agent made and only an agent can see is a change nobody can audit.

## Consequences

### Costs accepted

- **The first tools that write real state.** ADR-0008's "four read-only
  tools" is now four read-only note tools, two read-only cluster tools and
  three that write. The line it drew — an agent may not change what a note
  says — holds; the line it implied, that an agent changes nothing, does not.
- **Clusters are not synced.** ADR-0012 mirrors notes and tombstones. A
  cluster made on this machine stays on it: `evomem pull` will not bring it
  down and the PostgreSQL mirror will not carry it. Organising twice on two
  machines produces two unrelated sets of groups.
- **The browser cannot see them yet, and that is the point of the next piece
  of work.** The agent talks MCP to the Go binary and its store; the Flutter
  app has its own, and sync runs one way from the phone or browser into Go.
  So a grouping made by an agent is invisible exactly where this was supposed
  to be tried. Closing that needs a read path the Flutter app does not have —
  most likely an HTTP endpoint it already knows how to call, decided
  separately.
- **Nothing stops a cluster spanning projects' worth of notes badly.** There
  is no limit on size, no check that members share a project beyond the one
  the cluster names, and no measure of whether a grouping is any good. The
  person reading `evomem clusters` is the measure.

### Explicitly not decided here

- **How the browser shows clusters**, which is the next thing.
- **Syncing clusters**, which ADR-0012 would have to be extended for.
- **Automatic re-clustering** when notes arrive. Every grouping here is one an
  agent was asked to make.
