# ADR-0010: The ingestion endpoint fails closed, and listens on loopback

**Status:** accepted · **Date:** 2026-10-06

## Context

`core/api` accepts notes over HTTP. Two of its three endpoints have to be
reachable by Telegram and by Jira, which means something public in front of
it. It has no TLS, no rate limiting and no account model, and what it writes
to is read later by a language model.

## Decision

- **Loopback by default.** `DefaultAddr` is `127.0.0.1:8765`. Reaching it from
  outside is a tunnel the user puts in front of it, deliberately, rather than
  something that happens by leaving a laptop on a café network.
- **No secret, no endpoint.** `/ingest` is registered only with
  `EVOMEM_API_TOKEN`, `/telegram/webhook` only with a secret *and* a project,
  `/jira/webhook` only with a secret. A missing secret is a 404 at a known
  address, never an unauthenticated write. A server with nothing configured
  refuses to start.
- **Secrets from the environment, never from a flag.** A flag is visible in
  `ps` to every process on the machine.
- **Each endpoint uses its own scheme**, because the senders do:
  `/ingest` a bearer token, `/telegram/webhook` the
  `X-Telegram-Bot-Api-Secret-Token` header, `/jira/webhook` an HMAC-SHA256
  over the raw body in `X-Hub-Signature`. All compared in constant time; the
  HMAC with `hmac.Equal`.
- **Only sha256 for Jira.** The header's format carries a method prefix, and
  honouring a weaker one would let the sender pick the algorithm.
- **Bounded bodies.** 1 MiB on this package's own endpoints, 4 MiB in the Jira
  adapter, which has to buffer the body to verify it. Read, write and idle
  timeouts on the server.
- **`/healthz` is unauthenticated** and says only that the process is up. It
  is what a tunnel and a launch agent check.

## Consequences

- Every way of getting a credential wrong is a test:
  `TestIngestRejectsBadCredentials`, `TestRejectsWrongSecret`,
  `TestRejectsBadSignature`, `TestRejectsTamperedBody`,
  `TestEndpointsWithoutSecretsAreNotServed`.
- A webhook that is not storing anything is a 404 or a 403 the user can see,
  rather than a note that quietly never arrived.
- **Telegram retries.** An update it did not get a 2xx for is retried, and so
  is everything behind it. So a payload with nothing worth storing — a
  sticker, a bot's own message, an update type the adapter does not read — is
  answered 200 with nothing written. A non-2xx is reserved for what a retry
  could fix, which here is only a failing store.
- What is still missing: no TLS, no rate limiting, no replay window on the
  Jira signature (a captured delivery can be replayed until the secret
  changes), and no per-sender identity beyond the shared secret. All of that
  is the tunnel's job or a later decision.
