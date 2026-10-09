# ADR-0026: The panel is a screen in the web app, not a second UI

**Status:** accepted · **Date:** 2026-10-09

## Context

ADR-0025 built connections and left the panel out: "This decides what it
writes to and reads from. The screen is its own piece of work." A source is
connected from the command line today, which is not what "press sync in the
web platform" means.

## Decision

### 1. The panel is a screen in the Flutter app

Not a second UI served by the Go binary. The web app **is** the web platform
in this product's own terms: it already holds the server address and the
token, already reads notes and clusters over HTTP, and already has a settings
screen. A separate admin page would mean a second place to enter the same
address, a second thing to style, and a second answer to "where do I go".

### 2. Four routes, under the token everything else is under

```
GET    /connections            what is connected, and how the last pull went
POST   /connections            connect a source
DELETE /connections/{id}       forget a source and its token
POST   /connections/pull       pull from every source now
```

No token, no endpoint (ADR-0010). Limits, CORS and the bearer check are the
ones the read side already has (ADR-0024).

**A connection never carries its secret out.** The stored ciphertext is
unexported in the struct and absent from its JSON, so there is no response
shape that could leak it and no future field that accidentally does.

### 3. The token is entered in the browser and posted once

`POST /connections` takes the API token in the body. It crosses the browser,
the network and the server's memory before it is sealed.

This is the cost of a panel and it is accepted rather than hidden: the
alternative is the command line, which is what we are replacing. What
contains it is that the channel is the user's own server behind a bearer
token, CORS allows only origins they listed, and the token is sealed the
moment it arrives. What it does **not** survive is a server reached over
plain HTTP across a network somebody else can read — and `evomem serve`
offers no TLS by design (ADR-0010), so the tunnel in front of it is what
makes this safe.

### 4. Pulling is synchronous, and says so

`POST /connections/pull` runs the pull and answers when it is done. A source
with thousands of issues will hold the request open; the server's write
timeout is the limit.

A queue with a job id would be the grown-up answer and is not worth building
for one person pressing a button. When a pull routinely outlives the timeout,
that is the signal to build it — not before.

## Consequences

### Costs accepted

- **The token exists in a browser tab.** Named above. Anyone who can run
  script in that page while the form is filled can read it, which is the same
  exposure as any admin panel and is why the origin list is not a wildcard.
- **A long pull blocks a request**, and a browser that gives up does not stop
  the pull: it carries on server-side and records its result on the
  connection, so the screen catches up on the next load rather than losing
  it.
- **A fifth write route on one token.** `/ingest`, `/ingest/audio`, `PUT` and
  `DELETE /notes/{id}`, and now connections — which is the one that holds
  other people's credentials.

### Explicitly not decided here

- **The model API key for server-side clustering.** It belongs in the same
  keyring and is ADR-0023's amendment.
- **Editing a connection.** Forgetting it and connecting again is the whole
  of it today; a token that has to be replaced is replaced that way.
