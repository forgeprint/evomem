# ADR-0030: a token per device, and no login

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0010

## Context

The app has no sign-in screen, and somebody reasonably asked how one is
meant to work. The honest answer is that there is no account model and the
absence is the design, not an omission — ADR-0010 says so in as many words —
but the way the one credential is handled has outgrown it.

Today `EVOMEM_API_TOKEN` is a single string in the server's environment.
Every client holds the same one: the phone, the browser, an Apple Shortcut,
anything reading `GET /notes`. It is typed in by hand, it is never rotated,
and there is nothing to revoke. Lose a phone and the only remedy is changing
the token on the server, which signs out everything else at the same time.

## Decision

### 1. Evomem stays one person's

No accounts, no passwords, no sessions. This is somebody's own memory on
their own machine, and the reasons are not sentimental:

- **MCP does not go through HTTP at all.** An agent starts `evomem mcp` on
  the user's laptop and it opens the store directly. A sign-in on the HTTP
  side would lock one door and leave the other standing open.
- **The panel's sealed credentials belong to the installation**, not to a
  person: one Jira connection, one model key, one `EVOMEM_SECRET_KEY`.
- **`project_id` is a label, not a boundary.** Nothing in the schema or in
  any query treats it as one, and making it one is most of the work of
  multi-tenancy.

Multi-user is a different product. If it is ever wanted, this record is
where the reasoning to overturn goes.

### 2. A token per device, named and revocable

A `tokens` table: an id, a name somebody chose ("phone", "browser"), when it
was made, when it was last used, and when it was revoked.

The point is the unit of revocation. "Phone" and "browser" being different
strings means losing one costs one, and the server tells you which is which
because somebody named them.

### 3. The hash is stored, never the token

Unlike `connections`, which has to hand Jira its token back and therefore
seals it (ADR-0025), this side only ever needs to *verify*. So the row holds
a SHA-256 of the token and the token itself exists exactly once: in the reply
that created it. Lose it and you make another.

**SHA-256 and not bcrypt or argon2, deliberately.** Those exist to make
guessing a human-chosen password expensive. These tokens are 32 bytes from
`crypto/rand`; there is nothing to guess, and a slow hash on every request
would be a cost with no matching benefit. Compared in constant time, like
every other credential here.

A short prefix is kept in the clear so a person can tell one row from
another without the secret. A token reads `evm_<prefix>_<secret>`.

### 4. `EVOMEM_API_TOKEN` keeps working, and is what makes the first one

Otherwise there is no way to create the first device token, and every
existing install breaks on upgrade. It stays the environment's token: the
one that bootstraps, and the one a `curl` in a script uses.

So a request is authorised if it matches the environment's token **or** an
unrevoked row. The environment's is checked first because it is the common
case for the machine's own tooling.

### 5. Last used is recorded, at most once a minute

Knowing which token is actually in use is what makes revoking safe — "this
one has not been seen since March" is the difference between pruning and
guessing.

Writing on every request would be a write per request, and on SQLite that
goes through the single writer (ADR-0004). So it is updated only when the
stored value is more than a minute old. The cost is that "last used" can be
a minute stale, which is irrelevant for the question it answers.

## Consequences

### Costs accepted

- **This guards HTTP and nothing else.** Anybody with the database file, or
  with `EVOMEM_STORE_DSN`, has everything — tokens included, though not the
  tokens themselves. `evomem mcp`, `evomem add` and `psql` are all outside
  it. That is the shape of a single-user local store and it does not change
  here.
- **A revoked token is revoked at the next request**, not immediately. There
  is no session to tear down, which is also why there is nothing to leak.
- **One more table and one more lookup per request.** The lookup is a
  primary-key read on a table with a handful of rows.
- **`EVOMEM_API_TOKEN` is still a master key.** It cannot be revoked from the
  panel, only changed in the environment and restarted. Deliberate: a panel
  that could lock you out of your own server is worse.

### Explicitly not decided here

- **Accounts, for more than one person.** §1.
- **Scopes.** Every token can do everything the environment's can. A
  read-only token for a dashboard is a reasonable next thing and is not this.
- **Expiry.** Tokens live until revoked.
