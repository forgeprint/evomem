#!/usr/bin/env bash
# Compiles every release target with CGO_ENABLED=0. A pure-Go SQLite driver is
# the reason this can be done at all, and the reason it has to be checked: a
# dependency that quietly needs cgo would still build on the host.
set -euo pipefail
cd "$(dirname "$0")/.."

targets=(
	"darwin/arm64"
	"darwin/amd64"
	"linux/amd64"
	"linux/arm64"
	"windows/amd64"
)

for target in "${targets[@]}"; do
	os="${target%/*}"
	arch="${target#*/}"
	echo "==> ${os}/${arch}"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o /dev/null ./... 
done

echo "all targets compile without cgo"
