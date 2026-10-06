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

Four read-only tools: `search_notes`, `get_note`, `get_project_context`,
`list_projects`. See [docs/mcp.md](docs/mcp.md).

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
