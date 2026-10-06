#!/usr/bin/env bash
# Builds the host binary into dist/.
# CGO_ENABLED=0 is not optional: the core binary must need no C toolchain to
# build and no runtime on the user's machine.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${EVOMEM_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
ext="$(go env GOEXE)"

mkdir -p dist
CGO_ENABLED=0 go build \
	-trimpath \
	-ldflags "-s -w -X main.version=${version}" \
	-o "dist/evomem${ext}" \
	./cmd/evomem

echo "built dist/evomem${ext} (${version})"
"./dist/evomem${ext}" version
