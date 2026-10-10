# ADR-0035: the server records who asked

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0010

## Context

Ten notes were deleted through the API in half a second. The PostgreSQL
tombstones gave the identifiers and the moment — `13:47:17Z`, ten rows — and
that was the end of what could be known. The server kept no record of a
request ever arriving, so the question "by whom" had no answer anywhere, and
the only honest report was that I did not know.

Everything needed to answer it was already present and simply not written
down. Every write goes through one token check. A device token carries a
**name** the user chose (ADR-0030), precisely so one device can be told from
another when revoking. The check knew which device it had let in and threw
that away.

## Decision

### One line per request, on stderr, on by default

`EVOMEM_ACCESS_LOG=off` turns it off for somebody who would rather not keep
one. On by default, because the cost of keeping it is a line per request and
the cost of not keeping it was this.

```text
2026-10-10T14:27:03Z DELETE /notes/01M4… 401 13B 3ms from=172.20.0.1 as=-
2026-10-10T14:27:03Z GET /notes 200 279B 10ms from=172.20.0.1 as=environment
```

### What a line carries

- The method and **the path without its query.** A query string here carries
  the user's own content: `GET /notes?q=…` is the thing they were looking
  for, and a log is a second copy of it in a file nobody is thinking about.
  The cursors on `/sync/*` are lost with it; that is the price, and it is
  cheap next to keeping a search history by accident.
- **Who**, by the name on the device token, or `environment` for the token in
  the server's own configuration, or `-` for a request that carried no usable
  token. **Never the token, nor any part of it** — a log that leaks a
  credential is worse than no log, and a few characters of a secret are still
  characters of a secret. There is a test for both.
- The status, the bytes written, the duration, and the peer's address.

### Outermost, so refusals are recorded

The log wraps everything, including the CORS layer. A request that never
reaches a route — a 404, a refused token — is exactly the one somebody asks
about afterwards, and a log inside the authentication check would miss all of
them.

### `GET /healthz` is skipped

Compose checks it every ten seconds: eight thousand lines a day that answer
nothing. A log nobody can read is a log nobody reads.

## Costs

- **The server now keeps a record of its user's activity.** ADR-0010 says
  evomem has no account model and is one person's; this does not change that,
  but it does mean a file on the host that says when that person used their
  own memory and from where. It is on stderr, so where it ends up is
  whatever the host does with stderr — Docker keeps it in the container's log
  until the container is removed.
- **No rotation.** Nothing trims it. On a personal server at a handful of
  requests a day that is years of lines; a busy one would need `logrotate` or
  Docker's own `max-size`, and neither is configured here.
- **The query string is gone**, so a line cannot say which cursor a client
  was at. Deliberate, per above.
- **It says which token, not which human.** With one person and several
  devices that is the useful answer; it is not an audit trail and should not
  be described as one.

## A thing that was easy to get wrong

The log sits outermost and holds the request it was handed. The middleware
that learns the name builds a **new** request with `WithContext`. The outer
one never sees it, so the first version wrote `as=-` on every line including
ones it had just authorised. A pointer put into the context *before* the
request goes anywhere is shared by both layers. The test that caught it
asserts on the name, not merely on the line existing.
