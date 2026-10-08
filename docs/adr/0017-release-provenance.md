# ADR-0017: Releases are drafted by a tag-triggered workflow, with provenance

**Status:** accepted · **Date:** 2026-10-08

## Context

`scripts/release.sh` has existed since `b0a8d11`: it builds the five targets
with the version stamped into `main.version`, writes `SHA256SUMS`, verifies
the checksums against the bytes it just wrote, and then prints the publish
command rather than running it. It deliberately does not publish.

What was missing is the other half. `dist/` held `v0.1.0` binaries built on a
laptop, and nothing on GitHub produced or attested a release. A consumer who
downloaded a binary had no way to tell whose machine made it, from which
commit, or whether the checksum file beside it was written by the same party
as the binaries.

The options for the CI half:

1. **Nothing.** Keep publishing by hand with `gh release create` from a
   laptop's `dist/`. Zero machinery, and the bytes are whatever that laptop
   produced — unreproducible by anyone else, unattributable.
2. **Build in CI, publish automatically.** A tag becomes a public release with
   no human in between. Fast, and a mistyped tag is public before anyone reads
   it.
3. **Build in CI, draft the release, a person publishes.** The runner produces
   the bytes and attests them; publishing stays a human action.

## Decision

**Option 3**, as `.github/workflows/release.yml`, triggered on `v*` tags only.

The workflow calls `scripts/release.sh "${GITHUB_REF_NAME}"` — the same script
a developer runs, so CI and a laptop cannot drift. The script runs
`scripts/ci.sh` first, so a tag that does not pass the checks produces no
artifacts at all.

Three things follow from building on the runner rather than a laptop:

- **The version comes from the tag.** The workflow has no other input, so the
  string in `evomem version` is the ref it was built from. This is why the
  trigger is tags and not branches: a branch build would stamp something that
  cannot be pointed at later.
- **`actions/attest-build-provenance` can attest the artifacts**, because
  keyless signing needs the identity of the workflow that produced the bytes,
  which a laptop does not have. The attestation step runs *before* the upload:
  a failure there fails the job and there is no published release to correct
  afterwards. Its subject is `dist/*` — every artifact *and* `SHA256SUMS`,
  because a checksum file nobody can attribute proves nothing.
- **The release is a draft.** A person reads what is in it and presses publish.

### Costs accepted

- **Three permissions on one workflow**: `contents: write` to create the
  release, `id-token: write` and `attestations: write` to sign. This is the
  only workflow in the repository that can write anything; `ci.yml` stays
  `contents: read`. The token is minted per run and never leaves the runner.
- **Actions pinned to commit SHAs here, not tags.** `ci.yml` uses `@v7`,
  which is acceptable for a read-only workflow. This one can write a release,
  so a moved tag would be a moved release. The cost is that upgrades are
  manual and the pins go stale silently; the trailing `# v7.0.1` comments and
  the resolution date exist so that the next person can tell what a SHA was
  meant to be. SHAs were resolved against the actions repositories on
  2026-10-08.
- **Two builds per release.** `ci.sh` cross-compiles all five targets, then
  `release.sh` builds them again with the version flags. About a minute,
  against the alternative of a release-only build path that CI never
  exercises.
- **The draft is a manual step that can be forgotten.** A tag can sit with an
  unpublished draft indefinitely. Preferred over option 2, where the mistake
  is public instead of pending.

## Consequences

Cutting a release is now: `git tag -s vX.Y.Z -m "vX.Y.Z" && git push origin
vX.Y.Z`, then read the draft and publish it. `release.sh` still works offline
and still prints the by-hand path for when CI is unavailable.

There is **no install script** and no package-manager publishing. `SHA256SUMS`
is uploaded so that a person who downloads a binary can check it by hand, and
the attestation can be verified with `gh attestation verify`. If an installer
is added later it reads the checksum file from the release; that is a separate
decision and is not made here.
