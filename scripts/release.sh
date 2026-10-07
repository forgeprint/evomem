#!/usr/bin/env bash
# Builds every release artifact and the checksum file that proves which
# bytes they are.
#
# It does not publish. Publishing is a human action: the script ends by
# printing the command, so that whoever runs it has read what is in dist/
# first. A release that a script can create unattended is a release nobody
# looked at.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:-}"
if [ -z "$version" ]; then
	echo "usage: $0 <version>   e.g. $0 v0.1.0" >&2
	exit 1
fi
case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "a version is vMAJOR.MINOR.PATCH, such as v0.1.0; got $version" >&2
	exit 1
	;;
esac

# A release is cut from a clean tree. Anything else ships bytes that are not
# in the repository, and `git describe --dirty` would quietly bake that into
# the version string.
if [ -n "$(git status --porcelain)" ]; then
	echo "the working tree is not clean; commit or stash first" >&2
	git status --short >&2
	exit 1
fi

echo "==> checks"
./scripts/ci.sh > /dev/null

rm -rf dist
mkdir -p dist
echo "==> building $version"

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
	out="dist/evomem-${version}-${os}-${arch}"
	if [ "$os" = "windows" ]; then
		out="${out}.exe"
	fi
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags="-s -w -X main.version=${version}" -o "${out}" ./cmd/evomem
done

cd dist
# Sorted, so two builds of the same commit produce the same file.
artifacts=$(ls | sort)
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum $artifacts > SHA256SUMS
	verify() { sha256sum --check --quiet SHA256SUMS; }
else
	shasum -a 256 $artifacts > SHA256SUMS
	verify() { shasum -a 256 --check --status SHA256SUMS; }
fi

# The checksum file is checked here rather than trusted. A corrupt artifact
# is worth finding before it is published, not after.
echo "==> verifying"
verify || { echo "the checksums do not match what was built" >&2; exit 1; }
cd ..

echo
echo "dist/ holds $(echo "$artifacts" | wc -w | tr -d ' ') binaries and SHA256SUMS:"
sed 's/^/  /' dist/SHA256SUMS
echo
cat <<EOF
Read the above, then push the tag. That is the whole publish step: the
release workflow builds these same artifacts again on a clean runner and
drafts a release from them, for you to read and publish.

  git tag -s $version -m "$version"   # or -a if you do not sign tags
  git push origin $version

If CI is unavailable, the same release can be made by hand from what is in
dist/ right now:

  gh release create $version dist/* --draft \\
    --title "$version" --generate-notes

Either way SHA256SUMS has to be among the uploaded files: the install script
reads it from the release and refuses anything that does not match.
EOF