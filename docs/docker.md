# Running evomem continuously

Three containers: PostgreSQL, the server, and the web app. `compose.yaml` is
at the root of the repository.

```bash
cp .env.example .env
```

Fill in three values. The rest have working defaults.

| | |
| - | - |
| `POSTGRES_PASSWORD` | any long random string |
| `EVOMEM_API_TOKEN` | what the web app and the phone put in their settings |
| `EVOMEM_SECRET_KEY` | **exactly 32 bytes**; what seals the API tokens and model keys the panel stores |

A way to generate the first two, and a key of the right length:

```bash
openssl rand -base64 24
openssl rand -hex 16
```

Then:

```bash
docker compose up -d
```

- the web app on <http://localhost:8080>
- the server on <http://localhost:8787>
- PostgreSQL on `127.0.0.1:5432`

All three are published on loopback. Nothing here is reachable from the
network until you change that deliberately, and `evomem serve` offers no TLS
(ADR-0010) — put a tunnel or a reverse proxy in front of it if it has to be.

In the web app's settings, the server URL is `http://localhost:8787` and the
token is `EVOMEM_API_TOKEN`.

## What runs where

**PostgreSQL is where the server's memory is going** (ADR-0028). It is not
there yet: today the server keeps a SQLite file on its volume and
`docker compose run --rm sync` pushes the notes into PostgreSQL, which is the
mirror ADR-0012 describes. The container, the volume and the wiring are in
place so that the migration changes what the server reads and not how any of
this is run.

The phone keeps SQLite either way — that is the decision, not a stage — so it
works with no network, and pressing sync on the phone pushes what it holds up
to the server.

**`EVOMEM_SECRET_KEY` is worth keeping.** Losing it does not lose the notes,
but every API token and model key entered in the panel becomes unreadable and
has to be entered again.

## Things to know

```bash
docker compose logs -f server      # what it is serving, and what failed
docker compose run --rm sync       # push what the server holds into PostgreSQL
docker compose exec db psql -U evomem evomem
docker compose down                # stop; the volumes stay
docker compose down -v             # stop and delete the data
```

`docker compose down -v` deletes the notes. There is no undo and no backup:
a compose volume is where the data is, not a copy of it.

## The web image takes a while the first time

It downloads the Flutter SDK — checked against the sha256 Google publishes in
its release metadata — and builds the app. Google ships the Linux SDK for
**x64 only**, so on an arm64 machine that stage runs emulated and is slow.
Only that stage: the image that actually runs is nginx, and it is native. The
layer caches, so this is a once-per-SDK-bump cost.
