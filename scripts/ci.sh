#!/usr/bin/env bash
# The single entry point CI runs. The logic lives here, not in the workflow
# YAML, so the same checks run unchanged on a developer machine.
#
# Everything here works offline, with one exception: the first gitleaks run
# downloads a pinned, checksum-verified binary into .tools/. After that it is
# offline too.
#
# scripts/test-postgres.sh is deliberately not called: it needs a container
# runtime and a network every time.
set -euo pipefail
cd "$(dirname "$0")/.."

./scripts/test.sh
./scripts/crosscheck.sh
./scripts/gitleaks.sh
