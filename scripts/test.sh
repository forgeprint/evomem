#!/usr/bin/env bash
# Correctness checks: formatting, vet, tests.
# CI calls this script, so the same checks run unchanged on a developer machine.
set -euo pipefail
cd "$(dirname "$0")/.."

# vendor/ is committed byte-identical to what upstream published and is never
# formatted or vetted here.
go_files() {
	find . -name '*.go' -not -path './vendor/*' -not -path './.git/*'
}

echo "==> gofmt"
unformatted="$(gofmt -l $(go_files))"
if [ -n "$unformatted" ]; then
	echo "not gofmt'd:" >&2
	echo "$unformatted" >&2
	exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...
