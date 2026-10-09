# ADR-0028: PostgreSQL is the server's store; SQLite stays the phone's

**Status:** accepted · **Date:** 2026-10-09 · **Amends:** ADR-0001, ADR-0012

## Context

Until now there has been one store, SQLite, and PostgreSQL has been a mirror:
`core/sync` pushes notes into a single `evomem_notes` table so a second
machine can read them (ADR-0012). Clusters, connections, proposals, the
search index and the tombstones have never left the SQLite file.

The direction given is explicit:

> web uygulaması postgre db kullanacak, mobil uygulamalar offline çalışması
> için sqllite olacak, sync dediğimizde mobil veriler sunucu tarafındaki
> postgre'ye sync olacak

So the server — the thing the web app talks to, the thing that runs
continuously in Docker — keeps its memory in PostgreSQL. The phone keeps
SQLite because the phone has to work on a train.

## Decision

### 1. The server's store is PostgreSQL

Everything the server holds goes there: notes, the search index, clusters and
their membership, the sealed connections, the proposals, the tombstones and
the sync state. Not a mirror of the notes — the store.

`docker compose up` brings up PostgreSQL, the server and the web app, and the
server reaches the database over the compose network. The database's port is
published on loopback only: it is this machine's database, not the network's.

### 2. The phone keeps SQLite, and sync keeps its direction

Nothing about the mobile app changes. It writes sqflite locally, it works
with no network, and pressing sync pushes what it holds up to the server,
which now files it in PostgreSQL. The authority split from ADR-0024 is
untouched: a note belongs to the device that wrote it and is pushed one way;
a cluster lives on the server and is read over HTTP.

### 3. The SQLite implementation stays, and this is not optional

It would be tidier to delete it. Two things make that the wrong call, and
both are constraints that already exist rather than preferences:

- **`scripts/ci.sh` has to run offline on any machine.** That is why
  `scripts/test-postgres.sh` is deliberately separate today. A store that
  exists only against PostgreSQL means the whole store suite — the largest
  body of tests in this repository — needs a container runtime and a network
  pull before it can run at all.
- **`evomem mcp` and the command line are not the server.** An agent starts
  the MCP server on a laptop; `evomem add` is typed into a terminal. Making
  those require a running PostgreSQL turns a single static binary into a
  binary plus a database, which is the opposite of what ADR-0001 bought.

So the store backend is **chosen at runtime**: `EVOMEM_POSTGRES_DSN` set
means PostgreSQL, unset means the SQLite file. The server in Docker has it
set. A laptop, by default, does not.

**What this costs, and it is the main cost in this record:** every feature is
now written twice and has to be tested twice. A query that works on one and
not the other is a bug that only one half of the suite can see. This is the
price of the decision above, not a hedge against it.

### 4. One store type, two dialects — not two stores

`shared/database` keeps one `*DB` and gains a dialect. The alternative, a
`Store` interface with two implementations, means an interface of roughly
fifty methods and a change to all nine packages that take `*database.DB`
today. The differences between the two databases are narrower than that:

| what | SQLite | PostgreSQL |
| - | - | - |
| placeholders | `?` | `$1` |
| full text | FTS5 external-content table | `tsvector` column + GIN |
| metadata reads | `json_extract(metadata, '$.x')` | `metadata::jsonb ->> 'x'` |
| upsert | `INSERT OR REPLACE` | `ON CONFLICT … DO UPDATE` |
| concurrent writes | one writer, WAL (ADR-0004) | the server's own job |

Everything else is `TEXT`, `INTEGER` and ordinary SQL.

### 5. Turkish search is decided again, not assumed

ADR-0003 recorded what the SQLite search does and does not do: ğ/ş/ç/ö/ü fold,
`ı`/`i` do not, and there is no stemming. PostgreSQL has **no built-in Turkish
dictionary**, so `to_tsvector('turkish', …)` is not available and the honest
starting point is the `simple` configuration — which also does not stem.

That makes the two sides comparable rather than identical, and it is written
here so that nobody later reads a difference in results as a bug. Whether to
install a Turkish dictionary on the PostgreSQL side is a separate decision
with a separate cost: it makes the database no longer a stock image.

## Consequences

### Costs accepted

- **Local-first is now a property of the phone and the CLI, not of the
  server.** The server in Docker needs a database running beside it. ADR-0001
  bought "one binary, no runtime"; that still holds where it is typed, and no
  longer holds where it is served.
- **Two implementations of every query**, as in §3.
- **Search results will differ between the two backends**, as in §5.
- **ADR-0004's single-writer pool is dead weight on PostgreSQL** and has to be
  switched off there rather than merely left running: one connection to a
  server that handles concurrency is a bottleneck invented for nothing.
- **The migration is not one change.** It is the schema, the search, the
  metadata predicates, the upserts and the tests, and until it is finished
  the two halves are not equal.

### The order it gets done in

1. The dialect and the placeholder rewriting, with the existing SQLite suite
   still green — nothing behaves differently yet.
2. The schema and its migrations on PostgreSQL, with the store suite running
   against both and `scripts/test-postgres.sh` grown to cover it.
3. Search: `tsvector` and a GIN index, against ADR-0003's recorded behaviour
   so the difference is measured rather than discovered.
4. Clusters, connections, proposals and tombstones.
5. `core/sync` stops being the only thing that writes PostgreSQL. What
   becomes of `evomem_notes` — dropped, or kept as the mirror for a second
   machine — is decided then and not before.

### Explicitly not decided here

- **Whether the SQLite backend is eventually removed.** §3 says why it stays
  now; if the CLI and CI constraints ever change, this is revisited.
- **A Turkish dictionary on the PostgreSQL side** (§5).
- **Two-way sync.** The phone still pushes; the server still does not push
  back (ADR-0012, ADR-0024).
- **Connection pooling, read replicas, backups.** A `docker compose` volume
  is not a backup strategy and nobody should read it as one.
