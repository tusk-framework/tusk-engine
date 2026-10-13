# Engine releases and recovery

Stable Engine releases are calculated and published automatically from `main`.
The `workflow_run` event is only a wake-up signal: the trusted workflow resolves
the current `main` commit and proceeds only after the latest `Go`,
`Docker validation`, and `Skeleton smoke` runs all succeeded for that exact
SHA. A delayed event cannot publish an older commit.

## Version policy

The release calculator reads non-merge Conventional Commit messages since the
latest stable `vMAJOR.MINOR.PATCH` tag:

- `feat:` increments the minor version;
- `fix:` and `perf:` increment the patch version;
- `!` in the type/scope header or a non-empty `BREAKING CHANGE:` footer
  increments the major version and takes precedence over other changes;
- other types, such as `docs:`, `test:`, and `chore:`, do not create a release
  by themselves.

Malformed release-relevant commit messages fail closed. Pull request titles
are validated as Conventional Commits before merge; see
[`CONTRIBUTING.md`](../../CONTRIBUTING.md) for squash-merge and regular-merge
details.

## Required checks and outputs

All three required workflows must pass for the same current-main SHA. A failed,
pending, skipped, fork, pull-request, or stale-main run cannot create a tag.
The `main` Docker image is published after successful checks even when there is
no semantic release. A `v<version>` image is added only for a due release and
is built from the exact validated SHA. Before pushing the mutable `main` image,
the publisher checks that the source SHA is still the current `main` head.

The stable tag is annotated and immutable. In the same workflow run, the
publisher validates and signs the official toolchain catalog, verifies the
signature, creates catalog/provenance attestations, binds the catalog digest
into the binaries, and publishes checksums and platform artifacts. The
workflow uses the repository's `GITHUB_TOKEN`; it does not require a PAT.

## Failures and safe retry

Inspect the failed run in the repository's **Actions** tab. The relevant
workflows are **Go**, **Docker validation**, **Skeleton smoke**,
**Automatic release**, and **Release**. Fix failing checks or release
prerequisites first. The signing key is the `TUSK_CATALOG_SIGNING_KEY_B64`
Actions secret; `TUSK_CATALOG_KEY_ID` and
`TUSK_CATALOG_TRUST_ANCHORS_B64` are repository variables. Never paste key
material into a command, issue, log, or artifact.

If publication failed after a release tag was created, repair the cause and
rerun one of the required validation workflows for the same `main` commit from
the Actions UI. Its successful completion wakes the trusted aggregator again.
The aggregator recognizes the existing tag: an incomplete draft for the same
SHA can resume, a published release is an idempotent no-op, and a tag that
points to another SHA is never moved or overwritten. Do not push a release tag
or dispatch the publisher directly; those entry points are intentionally
disabled because they could execute workflow code from an untrusted ref.

If `main` advanced, the old SHA is not released as a new version. Wait for all
three required checks on the current `main` SHA; the next completed workflow
event can process that head. A completed published release is never modified
by an automatic retry. If it is missing an asset, stop and coordinate a
separate maintainer recovery rather than rerunning publication against it.

For diagnostics, inspect the individual run summaries and logs in Actions.
The release planner requires the latest run of each required workflow to be
successful, so a newer failed or pending rerun is not masked by an earlier
success. The versioned catalog release operations and signing-key rotation
process are documented in [`release/README.md`](../../release/README.md).
