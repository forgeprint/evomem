# Evomem

Open-source, AI-ready memory infrastructure. Local-first: one static Go binary
with no runtime dependencies, a local SQLite store, and a Model Context
Protocol server so coding agents can search and recall project context.

## Layout

```text
apps/web, apps/mobile   the Flutter and web clients
core/api, core/mcp      the HTTP entrypoint and the MCP server
shared/models           the data model
shared/database         the SQLite store
docs                    the plan and the decision records
```

## Using it from a coding agent

```sh
claude mcp add evomem -- /absolute/path/to/evomem mcp
```

Four tools read — `search_notes`, `get_note`, `get_project_context`,
`list_projects` — and `propose_note` suggests one. Nothing an agent proposes is
remembered until a person accepts it:

```sh
evomem review                 # what an agent suggested
evomem review -accept <id>    # store it as a note
```

See [docs/mcp.md](docs/mcp.md).

## Ingesting from elsewhere

```sh
evomem serve
```

`POST /ingest` for Apple Shortcuts and anything else, plus Telegram and Jira
webhook endpoints. Each needs its own secret in the environment, and an
endpoint whose secret is unset is not served. See [docs/api.md](docs/api.md).

## Syncing and archiving

```sh
evomem sync -init-remote   # create the mirror's schema
evomem sync                # push changes every five minutes
evomem archive -dry-run    # what would be thinned out
```

One-directional: the local file is the authority and PostgreSQL is a mirror.
Archiving holds back anything not yet synced. See [docs/sync.md](docs/sync.md).

## Building

```sh
./scripts/build.sh   # dist/evomem, CGO_ENABLED=0
./scripts/test.sh    # gofmt, go vet, go test
./scripts/ci.sh      # everything CI runs

./scripts/test-postgres.sh   # the sync transport, against a throwaway database
```

Go 1.26 or newer. Nothing else: dependencies are vendored, so a build needs no
network and no module cache.

## Contributing

Commits are signed off under the [DCO](DCO); `./scripts/ci.sh` is every check
CI runs. See [CONTRIBUTING.md](CONTRIBUTING.md), and
[SECURITY.md](SECURITY.md) for the threat model and how to report a
vulnerability privately.

Decisions live in [docs/adr](docs/adr). The ones worth knowing before reading
the code: [ADR-0002](docs/adr/0002-no-new-dependencies.md) on dependencies,
[ADR-0009](docs/adr/0009-tainted-content.md) on third-party content reaching a
model, and [ADR-0012](docs/adr/0012-one-way-sync.md) on why sync goes one way.

## License

[Apache License 2.0](LICENSE).
