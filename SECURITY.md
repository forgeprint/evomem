# Security Policy

## Reporting a vulnerability

Report privately through GitHub's [private vulnerability reporting](https://github.com/forgeprint/evomem/security/advisories/new)
on this repository. Please do not open a public issue for a security problem.

Include what you did, what happened, and the Evomem version (`evomem version`).
You will get an acknowledgement within a week.

## Threat model

Evomem stores a project's notes and feeds them back into a coding agent's
context through an MCP server. It also accepts notes over HTTP from Telegram,
from Jira and from Apple Shortcuts. That makes four things security-relevant.

**Persistent prompt injection.** A note an agent reads later is an
instruction channel, and the adapters fill the store with text third parties
wrote. A Jira description reading "ignore your previous instructions" is, to a
model reading `get_project_context`, indistinguishable from something the user
wrote. Nothing here can refuse to store such text — storing what arrives is
the adapters' whole job — so everything an adapter writes is marked `tainted`
with its origin, and the MCP tools carry that mark into the text the model
reads. See [ADR-0009](docs/adr/0009-tainted-content.md). This tells a reader
what it is holding; it does not guarantee the reader acts on it.

**The ingestion endpoints.** `evomem serve` has no TLS, no rate limiting and
no account model. It binds to `127.0.0.1:8765`, and reaching it from outside
is a tunnel the operator puts in front of it. Each endpoint carries its own
credential — a bearer token, Telegram's `X-Telegram-Bot-Api-Secret-Token`, an
HMAC-SHA256 over the raw body for Jira — all compared in constant time, and
an endpoint whose secret is unset is **not served at all**. See
[ADR-0010](docs/adr/0010-ingestion-endpoint-posture.md). Known gaps, worth
reading before relying on this: no replay window on the Jira signature, so a
captured delivery can be replayed until the secret is changed, and no
per-sender identity beyond the shared secret.

**Secrets on the command line.** Every credential — the API token, both
webhook secrets, the PostgreSQL connection string — is read from the
environment and never from a flag, because a flag is visible in `ps` to every
process on the machine.

**What leaves the machine.** Nothing, unless `evomem sync` is configured. The
binary makes no network calls of its own, sends no telemetry and runs no
background service. `sync` pushes notes to the PostgreSQL mirror the operator
points it at, and checks reachability by connecting to that host rather than
probing a third party — a probe elsewhere would answer a different question
and tell someone else that this user is online.

## Scope

In scope: any path that lets untrusted content reach a model without its mark,
any way to write to the store without the configured credential, anything that
writes a credential to disk or to a log, and anything in the sync engine that
can lose a local note.

Out of scope: vulnerabilities in the coding agents themselves, in the PostgreSQL
instance the mirror lives in, and in whatever tunnel is put in front of
`evomem serve`.
