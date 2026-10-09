#!/usr/bin/env bash
# Runs everything that needs a real PostgreSQL, against a throwaway database.
#
# Two things: the sync transport (ADR-0012), and the store itself against the
# PostgreSQL backend (ADR-0028). The second is the same suite ci.sh runs
# against SQLite — one set of assertions, both backends, because the only way
# to know the store behaves the same on each is to ask it the same questions.
#
# Deliberately not in ci.sh: that has to stay runnable offline on any machine,
# and this needs a container runtime and a network to pull the image.
set -euo pipefail
cd "$(dirname "$0")/.."

name="${EVOMEM_PG_CONTAINER:-evomem-pg-test}"
port="${EVOMEM_PG_PORT:-55432}"
image="${EVOMEM_PG_IMAGE:-postgres:17-alpine}"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cleanup
echo "==> starting $image on port $port"
docker run -d --rm --name "$name" \
	-e POSTGRES_PASSWORD=test -e POSTGRES_DB=evomem_test \
	-p "$port":5432 "$image" >/dev/null

echo "==> waiting for it to accept connections"
for _ in $(seq 1 60); do
	if docker exec "$name" pg_isready -U postgres -d evomem_test >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

dsn="postgres://postgres:test@127.0.0.1:${port}/evomem_test?sslmode=disable"

echo "==> the sync transport"
EVOMEM_TEST_POSTGRES_DSN="$dsn" go test ./core/sync/ "$@"

echo "==> the store, on PostgreSQL"
# Each test gets a schema of its own and drops it afterwards, so these run in
# one database without seeing each other.
EVOMEM_TEST_POSTGRES_DSN="$dsn" go test ./shared/database/ "$@"

echo
echo "Tests that cannot run here say so and why: the full-text search is"
echo "still FTS5-only (ADR-0028 step 3), and a handful assert things that are"
echo "true of a SQLite file and not of a server. Run with -v to read them."
