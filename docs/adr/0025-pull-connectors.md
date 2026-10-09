# ADR-0025: The hub pulls from somebody else's source, with a key it keeps

**Status:** accepted · **Date:** 2026-10-09 · Amends ADR-0010.

## Context

Sync means two different things depending on whose source it is
(ADR-0024 §0):

- **A source I own pushes.** The phone sends what it holds when a person asks.
  Built.
- **A source somebody else owns is pulled.** Asking the platform to sync Jira
  means the platform calling *their* API.

Only the first exists. Jira and Telegram reach evomem by **webhook** today:
they push, on their schedule, if somebody configured them to. Pressing "sync"
in a panel and having Jira's issues arrive is a different mechanism, and it
needs three things the project does not have — somewhere to keep per-source
credentials, a cursor per source, and something to run the loop.

### The posture this argues with

ADR-0010 says **secrets come from the environment, never from a flag**,
because a flag is visible in `ps`. The reasoning was about *process*
arguments, and it assumed one operator configuring one server before starting
it. A panel where a person enters an API token for each source they connect
cannot work that way: the server is already running, and the next source is
added without restarting it.

## Decision

### 1. A connection is a row, and its secret is encrypted in that row

```
connections(id, source_type, project_id, base_url, account, secret, cursor,
            last_pulled_at, last_error, created_at, updated_at)
```

`secret` holds **ciphertext**, never the token. It is sealed with a key read
from `EVOMEM_SECRET_KEY` — from the environment, as ADR-0010 requires — using
AES-256-GCM from the standard library, with a random nonce per write stored
alongside the ciphertext.

What this buys and what it does not:

- A stolen database file is not enough. The key lives in the environment of
  the process, not in the file that holds the notes.
- It is **not** protection from somebody who has the machine. They have the
  environment too. The threat this answers is a copied `evomem.db`, a backup,
  a synced folder — which is the likely way this file leaves.
- **No key, no connections.** Without `EVOMEM_SECRET_KEY` the rows cannot be
  read and the pull does nothing, saying so. Losing the key means re-entering
  every token, and nothing here can recover one.

### 2. A `Puller` per source, and Jira is the first

```go
type Puller interface {
    Pull(ctx context.Context, conn Connection, since string) (notes []*models.Note, cursor string, err error)
}
```

The cursor is the source's own idea of where we are — a page token, a
timestamp, whatever that API gives — kept as an opaque string, because a
cursor this code interprets is a cursor that breaks when the vendor changes
it.

**Everything a puller brings in is tainted** (ADR-0009). It is somebody
else's text arriving without anyone having read it, which is exactly what the
mark exists for.

### 3. Jira's wire format, read from Atlassian's own reference

Checked 2026-10-09 against
<https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/>
and
<https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/>:

- `GET /rest/api/3/search/jql` with query parameters `jql`, `nextPageToken`,
  `maxResults` and `fields`. The older `/rest/api/3/search` is still listed,
  and is not what this uses.
- The answer carries `issues`, `isLast` and `nextPageToken`.
- Authentication is **Basic** with the account's **email** in the user
  position and an **API token** in the password position; the page states
  that password authentication is deprecated.

Two things the community reports and the vendor does not, which this code
guards against rather than trusting:

- Sending `nextPageToken` on the **first** request can be rejected as invalid.
  The first call omits it.
- `isLast` is not always reliable and a token can repeat, so the loop stops on
  an absent token, on a repeated one, and at a page limit.

### 4. Pulling is a command, triggered by a person

`evomem connect` manages connections and `evomem pull-sources` runs them.
Nothing pulls on a timer yet: "press sync" is what was asked for, and a
connector that runs unattended is a connector that burns somebody's API quota
while they sleep.

A failure is recorded on the connection — `last_error`, `last_pulled_at` —
rather than only logged, for the same reason a failed transcription is
recorded on its note: a log has rotated by the time somebody asks.

## Consequences

### Costs accepted

- **Evomem now holds other people's credentials.** It was a store of notes;
  it is now also a keyring. A token here reaches whatever that token reaches —
  a Jira token can usually read every issue the person can — and nothing in
  this project can reduce that scope.
- **A key to manage.** `EVOMEM_SECRET_KEY` has to exist, be the same across
  restarts, and survive a machine move, and losing it means re-entering every
  token. There is no rotation path: changing the key makes every stored secret
  unreadable.
- **A deliberate amendment to ADR-0010.** Secrets still come from the
  environment — but now one key does, and the per-source tokens come from a
  person at runtime and are kept. That is a wider surface than "configure the
  process before it starts", and it is what a panel means.
- **One vendor's API is now this project's problem.** Jira's search endpoint
  changed once already; the guards above exist because of it. Every connector
  adds a surface that can break without any change here.

### Explicitly not decided here

- **The panel itself.** This decides what it writes to and reads from. The
  screen is its own piece of work.
- **Apple Notes.** It has no public cloud API; reaching it means Shortcuts
  pushing to `/ingest`, which is the push half and already works.
- **Scheduled pulls.** Named above.
- **Server-side clustering from a model API key.** It needs the same keyring
  and is the obvious next user of it, but it is ADR-0023's amendment, not
  this one.
