#!/usr/bin/env bash
# Runs the PostgreSQL transport's tests against a throwaway database.
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

echo "==> go test"
EVOMEM_TEST_POSTGRES_DSN="postgres://postgres:test@127.0.0.1:${port}/evomem_test?sslmode=disable" \
	go test ./core/sync/ "$@"
