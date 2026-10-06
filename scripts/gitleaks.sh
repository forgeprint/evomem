#!/usr/bin/env bash
# Secret scan with a pinned gitleaks binary.
#
# Not the official GitHub Action, on purpose:
#   - the action requires GITLEAKS_LICENSE for organisation accounts;
#   - a downloaded, checksum-verified binary behaves identically here and in CI.
#
# Bumping the version means replacing BOTH the version and the checksums below,
# taken from the release's own gitleaks_<version>_checksums.txt.
set -euo pipefail
cd "$(dirname "$0")/.."

GITLEAKS_VERSION="8.30.1"
TOOLS_DIR=".tools/gitleaks-${GITLEAKS_VERSION}"

# sha256, from https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_checksums.txt
checksum_for() {
	case "$1" in
	darwin_arm64) echo "b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5" ;;
	darwin_x64)   echo "dfe101a4db2255fc85120ac7f3d25e4342c3c20cf749f2c20a18081af1952709" ;;
	linux_arm64)  echo "e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080" ;;
	linux_x64)    echo "551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb" ;;
	windows_x64)  echo "d29144deff3a68aa93ced33dddf84b7fdc26070add4aa0f4513094c8332afc4e" ;;
	*) return 1 ;;
	esac
}

detect_platform() {
	local os arch
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	MINGW* | MSYS* | CYGWIN*) os=windows ;;
	*)
		echo "unsupported OS: $(uname -s)" >&2
		return 1
		;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=x64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*)
		echo "unsupported architecture: $(uname -m)" >&2
		return 1
		;;
	esac
	echo "${os}_${arch}"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo "no sha256 tool found (need sha256sum or shasum)" >&2
		return 1
	fi
}

install_gitleaks() {
	local platform="$1" expected archive url actual
	expected="$(checksum_for "$platform")" || {
		echo "no pinned checksum for platform ${platform}" >&2
		return 1
	}

	if [ "${platform%_*}" = "windows" ]; then
		archive="gitleaks_${GITLEAKS_VERSION}_${platform}.zip"
	else
		archive="gitleaks_${GITLEAKS_VERSION}_${platform}.tar.gz"
	fi
	url="https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/${archive}"

	mkdir -p "$TOOLS_DIR"
	echo "==> downloading ${archive}"
	curl -fsSL --retry 3 -o "${TOOLS_DIR}/${archive}" "$url"

	actual="$(sha256_of "${TOOLS_DIR}/${archive}")"
	if [ "$actual" != "$expected" ]; then
		echo "checksum mismatch for ${archive}" >&2
		echo "  expected ${expected}" >&2
		echo "  actual   ${actual}" >&2
		rm -f "${TOOLS_DIR}/${archive}"
		return 1
	fi
	echo "==> checksum ok"

	case "$archive" in
	*.tar.gz)
		tar -xzf "${TOOLS_DIR}/${archive}" -C "$TOOLS_DIR" gitleaks
		;;
	*.zip)
		if command -v unzip >/dev/null 2>&1; then
			unzip -oq "${TOOLS_DIR}/${archive}" gitleaks.exe -d "$TOOLS_DIR"
		else
			# Git Bash ships no unzip; PowerShell is always present on Windows.
			powershell -NoProfile -NonInteractive -Command \
				"Expand-Archive -Force -LiteralPath '${TOOLS_DIR}/${archive}' -DestinationPath '${TOOLS_DIR}'"
		fi
		;;
	esac
	rm -f "${TOOLS_DIR}/${archive}"
}

platform="$(detect_platform)"
bin="${TOOLS_DIR}/gitleaks"
if [ "${platform%_*}" = "windows" ]; then bin="${bin}.exe"; fi

if [ ! -x "$bin" ]; then
	install_gitleaks "$platform"
fi

echo "==> $("$bin" version) scanning working tree"
"$bin" dir . --config .gitleaks.toml --redact --no-banner
