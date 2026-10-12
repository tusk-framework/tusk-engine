# Automated Engine Releases — Design Specification

## Context

The Engine documents release automation as driven by Conventional Commits, but
the current `.github/workflows/release.yml` only runs after a `v*` tag is
pushed. The main-branch workflows validate Go build/tests/vet, Docker build,
and the generated application skeleton. This leaves version calculation and
tag creation as a manual gap between a green `main` and the signed GoReleaser
publication.

The existing release pipeline validates the official toolchain catalog,
signs and verifies it, creates GitHub attestations for the catalog and its
provenance, binds the catalog digest into binaries, and publishes with
GoReleaser. These trust and provenance steps are required behavior and must be
preserved.

## Goals

- Publish a new Engine release automatically after the complete required CI
  suite passes for `main`.
- Select semantic version increments deterministically from Conventional
  Commits since the last stable version tag.
- Run versioning, tag creation, signing, attestations, and GoReleaser
  publication in a workflow execution that does not depend on a tag push
  triggering another workflow.
- Preserve catalog validation, signing, trust anchors, checksums, and release
  artifacts already produced by the current release pipeline.
- Avoid introducing a long-lived PAT or other additional credential when the
  existing repository `GITHUB_TOKEN` and workflow permissions suffice.
- Ensure concurrent or repeated main pushes cannot publish competing or
  duplicate releases.

## Non-goals

- Automatically releasing the Framework, app, or docs repositories as part of
  this Engine change.
- Replacing GoReleaser, catalog signing, GitHub attestations, or the current
  release artifact formats.
- Publishing for documentation, tests, chores, or other changes that do not
  imply a public version increment.
- Releasing unvalidated commits, pull-request branches, or arbitrary tags.

## Release policy

Use the latest stable `vMAJOR.MINOR.PATCH` tag as the comparison point and
consider non-merge Conventional Commit messages reachable from the current
`main` head but not from that tag. The Engine history contains both squash
commits and synthetic GitHub merge commits. Ignore synthetic merge wrapper
messages (for example, `Merge pull request #...`) while retaining commits
inside those merges; this prevents GitHub's merge metadata from being
misclassified as malformed input. For squash merges, the validated PR title
is the canonical release signal and the squash commit subject must retain it.
For regular merge commits, the PR title is validated before merge and the
individual commits are classified; contribution rules require those commit
messages to use Conventional Commits as well.

Increment rules:

- `feat` produces a minor release.
- `fix` and `perf` produce a patch release.
- Any valid `BREAKING CHANGE:` footer or `!` marker on a Conventional Commit
  produces a major release, taking precedence over all other types.
- Other types do not independently trigger a release.
- If the range contains no release-triggering change, finish successfully
  without creating a tag or release.
- When multiple release-triggering commits exist, choose the highest
  applicable increment: major, then minor, then patch.

Repository contribution guidance already states these semantics. Pull-request
titles must be validated before merge as Conventional Commits, including valid
breaking-change notation. The release calculator must fail closed on malformed
release-relevant metadata rather than silently guessing a version.

## Workflow architecture

The main-branch release orchestrator must gate on successful Go
build/test/vet, Docker build validation, and Skeleton smoke runs for the same
`main` commit. These checks currently live in separate workflows, so the
orchestrator must aggregate their completed workflow runs by exact head SHA;
it must not assume GitHub Actions `needs` can refer across workflows. It must
verify each run belongs to this repository, was triggered by a push to
`main`, targets the candidate SHA, and concluded successfully. The Docker job
currently also pushes the mutable `main` image on every main push; that image
push is not a release gate and must not be mistaken for validation. Immutable
versioned image publication, if included in this release, must use the
calculated version and validated commit. A skipped or failed required
validation run is not a green result.

After the workflow-run aggregator confirms all required validation runs have
succeeded for the same commit, a serialized release job will:

1. Check out the exact `main` commit that passed the required checks and fetch
   complete tag/history data.
2. Determine the latest stable version tag and calculate the next version
   from the Conventional Commit range.
3. Exit without publishing if no release-triggering commits exist.
4. Verify that the target tag does not already exist and that the commit being
   released is still the intended validated `main` commit.
5. Create and push the version tag with the job-scoped `GITHUB_TOKEN`.
6. In the same workflow run, perform the existing catalog validation,
   signature generation and verification, attestations, digest binding, and
   GoReleaser publication for that exact tag.
7. Publish any versioned Docker image only after release version calculation,
   tagging, and successful image build for the same source commit. The existing
   mutable `main` image behavior may continue independently, but its outcome
   must neither satisfy nor bypass required release validation.

Tag creation must not be used as an event hand-off to a separate workflow:
   GitHub suppresses most workflow runs caused by events generated with
   `GITHUB_TOKEN`. Tag-triggered manual/compatibility releases may remain
   supported, but both entry paths must call the same release implementation
   to prevent policy drift. A tag-triggered run must validate that it is a
   stable version tag and build/publish that exact tag. GitHub reusable
   workflows execute as jobs rather than steps, so the shared publisher must
   be called as a reusable workflow job, composite action, or repository-local
   script; do not attempt to invoke a reusable workflow as a step.

The release job must use the narrowest permissions that support tag creation,
GitHub release upload, OIDC-backed attestations, and package publication only
where needed. Signing material remains available solely to release steps that
need it; it must never be printed or persisted as an artifact.

## Concurrency, idempotency, and failure behavior

- Allow at most one automatic release job for this repository at a time; do
  not cancel an in-progress release in favor of a newer push.
- Before tag creation, re-check the remote tag and current `main` SHA to handle
  queued runs and manual tag races. If `main` advanced while this run waited,
  do not release a stale commit; the newest successful main run can calculate
  the accumulated range.
- A pre-existing target tag is a hard error unless the run is an intentional
  retry for the same commit and the release is not already complete. Never
  move or overwrite tags/releases.
- Any validation, signing, attestation, or GoReleaser failure leaves the run
  failed and must not report a successful release. The immutable tag may
  remain; documented retry behavior must resume or safely recreate the GitHub
  release for the same tag without changing its target commit.
- A commit that has already been released must not create another version if
  the workflow is rerun.

## Security and provenance

- Do not add a PAT solely to trigger the release workflow.
- Grant `contents: write`, `id-token: write`, and `attestations: write` only
  to the job that needs them; keep validation jobs read-only.
- Continue validating the downloaded official catalog payload and checking
  required signing inputs before use.
- Continue signing and verifying the catalog before creating attestations and
  binding its SHA-256 digest into the binaries.
- Do not expose secret values in logs, job outputs, artifacts, or release
  metadata.

## Acceptance criteria

1. A green main run containing a `feat` since the latest stable tag publishes
   exactly one minor version.
2. A green main run containing only `fix`/`perf` release commits publishes
   exactly one patch version.
3. A breaking change publishes a major version even if the range also contains
   features or fixes.
4. Non-release commit types alone create no tag and no GitHub release.
5. A failed or skipped required CI job cannot create a release tag or publish
   artifacts.
6. Concurrent main updates cannot release an outdated commit or duplicate a
   version.
7. Manual reruns and pre-existing tags never move tags or duplicate a
   completed release.
8. The automatic release contains the signed catalog, catalog provenance,
   attestations, checksum file, and platform binaries produced by GoReleaser.
9. The release uses the same validated commit as the required successful CI
   run, and its generated binaries embed the digest of the signed catalog
   included in that release.
10. No PAT or new long-lived secret is required.
11. Pull requests with invalid Conventional Commit titles are rejected before
    merge, and current contribution documentation matches the enforced rules.
12. CI tests cover version classification, no-release ranges, breaking-change
    precedence, malformed messages, pre-existing tags, main advancing during
    queueing, and safe retry after partial publication failure.
13. Docker validation and mutable `main` image publication are distinct; a
    required Docker validation failure blocks a release, and any versioned
    image tag points to the exact released commit and calculated version.

## Open design detail for implementation planning

The implementation plan must select a maintained version-calculation method
and define its exact parsing contract, test strategy, and safe retry sequence.
It must not weaken the policy or acceptance criteria above. Prefer a
repository-local or pinned tool/action over an unpinned `latest` dependency;
document how tool upgrades are reviewed.
