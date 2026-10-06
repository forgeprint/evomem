# Contributing to Evomem

## Sign your commits off (DCO)

This project uses the [Developer Certificate of Origin](DCO). Every commit must
carry a `Signed-off-by` line matching the author:

```sh
git commit -s -m "your message"
```

There is no CLA. The sign-off is the whole agreement.

## Dependencies

Evomem depends on the Go standard library, `modernc.org/sqlite` and
`github.com/jackc/pgx/v5`, and nothing else — including for tests. Both are
vendored, so a build needs no network and no module cache, and both are pure
Go, so it needs no C compiler.

If a change appears to need another dependency, open an issue first. If it is
accepted, it comes with an ADR in `docs/adr/` explaining why, and the module
is vendored.

`vendor/` is committed on purpose. It is exempt from line-ending
normalisation in `.gitattributes` so the tree stays byte-identical to what
upstream published, and `scripts/test.sh` excludes it from the format check.
**Never run `gofmt -w .` from the repository root** — it rewrites vendored
doc comments across the tree and no check will catch it. Format a directory
you own instead: `gofmt -w ./core`, `gofmt -w ./shared`.

## Before you open a pull request

Run the same checks CI runs:

```sh
./scripts/ci.sh
```

That is formatting, `go vet`, tests, a cross-compile of every release target
with `CGO_ENABLED=0`, and a secret scan. CI calls this exact script, so a
green run locally means a green run there.

The PostgreSQL transport has its own tests, which need a database and are
skipped without one:

```sh
./scripts/test-postgres.sh
```

They are not in `ci.sh`, which has to stay runnable offline.

## Never write an external API detail from memory

Before writing code against any third-party payload shape, header name, field
name, error code or protocol revision — a webhook body, a bot API, MCP, an
OAuth flow — check that system's current official documentation and cite the
page and the date in the code.

This is not pedantry. A field name guessed wrong fails silently: an absent
field decodes as a Go zero value, not an error, so the mistake surfaces as
missing notes weeks later rather than as a failed build. Every external shape
in this repository carries the date it was checked, and the ones that look
surprising are the ones that were checked.

## Every new adapter marks its content tainted

An adapter writes text somebody outside the project wrote, and that text
later reaches a model's context. `Note.MarkTainted(origin)` is what tells the
reader so. There is no way to enforce it from the model package, which makes
it a review item on every adapter: a new one that forgets the call silently
loses the property. See [ADR-0009](docs/adr/0009-tainted-content.md).

## Decisions are recorded, not re-argued

Anything architectural goes in `docs/adr/` with the costs named. If a
recorded decision looks unworkable, say why in an issue rather than changing
it in a pull request.

`docs/ilerleme.md` is the notebook between working sessions — what was done,
what was decided, what is open. It and `docs/plan.md` are in Turkish;
everything else, including code and comments, is English.

## Do not commit secrets

`scripts/gitleaks.sh` runs as part of `ci.sh`, with a pinned,
checksum-verified binary. Nothing in this repository should contain a real
token, webhook secret or connection string, including in a test fixture.
