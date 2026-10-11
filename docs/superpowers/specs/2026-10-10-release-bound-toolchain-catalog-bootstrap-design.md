# Release-Bound Toolchain Catalog Bootstrap

## Status

Proposed design. This document defines behavior for a future implementation;
it does not change the current CLI or release process.

## Context

The Engine can already verify the official signed toolchain catalog, and
released Engine binaries embed the catalog trust anchors. The official catalog
and provenance are published as assets alongside Engine releases. However,
`tusk setup --toolchain` currently requires users to place
`.tusk/toolchain.catalog.json` in the project themselves. This breaks the
otherwise guided setup flow and makes the public catalog release asset hard to
discover.

The Engine also already provisions only explicitly pinned tools. A pin such as
`roadrunner@2025.1.15` is stored in `.tusk/toolchain.json`; setup then verifies
the catalog and artifact before installing project-local files. This design
keeps those explicitness and verification guarantees while removing the manual
catalog-fetch step.

## Goals

- Let an online `tusk setup --toolchain` obtain the official catalog matching
  the running Engine release when there are tools to provision.
- Verify the catalog with the trust anchors embedded in that Engine binary
  before persisting or using it.
- Keep offline behavior deterministic: no network access, only an already
  cached and verified catalog and artifacts.
- Preserve explicit version pins and project-local installation; never infer
  versions or install tools from `start`, `doctor`, or ordinary CLI use.
- Provide actionable failures and leave the last known-good catalog intact if
  a new download is invalid or incomplete.

## Non-goals

- Bundling RoadRunner into the Engine binary or making the Engine a fork of
  RoadRunner.
- Automatically choosing or upgrading a RoadRunner, PHP, or Composer version.
- Installing tools globally, changing the user's `PATH`, or installing OS
  packages.
- Making `composer create-project` independent of PHP and Composer. This flow
  only manages catalog-backed toolchain artifacts after the project and its
  explicit pins exist.
- Expanding managed PHP or extension coverage beyond the official catalog's
  supported targets.
- Fetching catalogs from a moving `latest` endpoint, arbitrary project URLs,
  or a user-controlled catalog source.

## User experience

The normal project flow is explicit and reproducible:

```sh
tusk toolchain pin roadrunner@2025.1.15
tusk setup --toolchain
tusk doctor
tusk build
tusk start
```

The pin command continues to update `.tusk/toolchain.json` without network
access. Online setup discovers the catalog for its own Engine release, verifies
it, then provisions only the requested pinned tools. The project-local
`.tusk/` directory remains generated state and should not be committed merely
to distribute the catalog cache.

For consumers of the official Tusk skeleton, its setup guide should document
the same pin-then-setup sequence and explain the PHP/Composer prerequisite for
creating the project. That consumer documentation change is rollout work; this
Engine design does not assume that the skeleton bundles a catalog or runtime.

## Catalog discovery and version binding

Release builds already inject `documentationVersion` from GoReleaser's
`{{ .Version }}` value. The implementation should expose one authoritative
release version to the CLI rather than independently deriving versions in
multiple packages. It should normalize the release tag convention (for
example, `v0.3.2`) into the exact version required to locate the matching
Engine release asset:

`https://github.com/tusk-framework/tusk-engine/releases/download/v<version>/toolchain-catalog.json`

Only a valid stable release version may be used to construct this URL. A
development/snapshot build (`dev`, empty, or prerelease unless explicitly
supported by a future design) must not guess a release or silently use the
latest catalog. It should fail with guidance to install a released Engine or
configure a trusted catalog through the documented development path.

The release pipeline computes the SHA-256 digest of the signed catalog after
signing and injects that digest into every Engine binary built for the same
tag. The binary therefore carries both the public signing anchors and the
expected digest for its exact catalog asset. `catalog_version` remains the
catalog inventory's own version; it is not the Engine release version and
must not be compared to it. This distinction preserves the existing catalog
versioning scheme while binding each Engine build to the exact signed catalog
published with its tag.

## Online setup behavior

1. Parse and validate setup options and the project toolchain manifest.
2. Determine the requested pinned tools using the existing setup rules. If
   there are no tools to provision, preserve the existing no-op behavior and
   do not make a catalog request.
3. Construct the exact release asset URL from the running Engine version.
4. Download the catalog over HTTPS with bounded time and response size. Permit
   only the canonical GitHub release URL and the documented HTTPS redirect
   destinations needed for GitHub release assets; reject downgrade to HTTP,
   unexpected hosts, and redirect loops.
5. Verify the fetched bytes' SHA-256 against the digest embedded in this Engine
   build, then verify the complete catalog using `OfficialCatalogVerifier`,
   including its signature, validity window, supported schema, and artifact
   signatures.
6. Persist the verified bytes atomically to
   `.tusk/toolchain.catalog.json`. Never truncate or replace the existing file
   before every check succeeds.
7. Run the existing setup service, which independently loads/verifies the
   catalog against the same required release digest and trust anchors before
   provisioning the explicitly pinned artifacts. Artifact download
   digests/signatures and manifest atomicity remain enforced as today. The
   setup service must fail closed if the expected release digest is absent or
   malformed; signature-only verification is not sufficient for provisioning.

If catalog acquisition or validation fails, setup stops before artifact
provisioning and reports an actionable error. An online attempt must not
silently fall back to an older cached catalog; users who intentionally need
cache-only behavior must select `--offline`.

## Offline setup behavior

With `--offline`, catalog resolution must not create an HTTP client, perform
DNS, contact GitHub, or follow any URL. The Engine loads the project-local
catalog, checks its bytes against the digest embedded in this Engine build,
verifies it with the embedded anchors and validity rules, and proceeds only
if the exact release catalog and required artifacts are already available
locally. If the catalog is missing, expired, invalid, or has another digest,
fail with a message explaining how to run online setup with the matching
Engine release. Do not replace or repair the offline cache from the network.

## Trust, storage, and failure safety

- The trust root remains the public Ed25519 anchors embedded in official
  Engine releases. Downloading a catalog never makes it trusted by itself.
- Official release binaries also embed the SHA-256 digest of their exact
  signed catalog asset; release setup checks this digest in online and offline
  modes. `catalog_version` is independent metadata and is not an Engine tag.
- Catalog signature and validity checks happen before the downloaded bytes
  become the project cache or influence artifact selection.
- Enforce a small explicit maximum catalog size and a finite request timeout;
  reject empty, oversized, malformed, or trailing data.
- Store through a temporary file in the `.tusk` directory, flush and close it,
  then atomically rename it into place. Clean up the temporary file on errors.
- On failed refresh, preserve any existing catalog unchanged. Do not use that
  stale file for the current online setup attempt.
- Do not log catalog contents, authorization headers, or environment secrets.
  The endpoint is public and must not require a GitHub token.
- Catalog acquisition must be injectable in tests so they can exercise
  redirects, bounds, timeouts, and failures without external network access.

## Compatibility and failure messages

The `tusk setup --toolchain` command and `--offline` flag remain unchanged.
Existing projects with a verified catalog continue to work offline while that
catalog remains valid and its bytes match the Engine release's embedded
digest. Projects do not need to commit catalog bytes or downloaded tools.

Errors should distinguish at least:

- no pinned tool requests (successful no-op, no network);
- development/snapshot Engine with no release catalog identity;
- missing embedded official trust anchors;
- release asset unavailable or network/timeout/size failure;
- invalid signature, invalid validity window, expired catalog, or catalog
  digest mismatch;
- offline mode with missing/invalid catalog or missing cached artifact; and
- pinned version/platform absent from the authenticated catalog.

Each error should identify the corrective action without suggesting unsafe
workarounds such as disabling signature checks or using an arbitrary URL.

## Validation strategy

Automated tests should cover:

- exact release URL and normalized version handling, including `dev`, malformed
  versions, and prerelease policy;
- online acquisition for a pinned request and no request for an empty set of
  pins;
- offline mode making zero network calls;
- signed catalog acceptance and rejection of mismatched catalog digests;
- missing embedded digest rejection;
- missing anchors, invalid signature, expired catalog, malformed/oversized
  response, non-HTTPS redirect, unexpected redirect host, timeout, and 404;
- atomic cache replacement only after verification, preserving an existing
  catalog after every failure point;
- successful handoff to the existing provisioner and no artifact install when
  catalog verification fails; and
- CLI exit code and actionable output for setup failures.

Tests must use an injected HTTP transport/server and generated test keys, never
the production trust key or the public network. Existing catalog and
provisioning tests remain in force. The implementation PR should run the
Engine's Go test, vet, build, and diff checks, plus the CI suite.

## Rollout

1. Review and approve this design.
2. Write an implementation plan that separates the release-version source,
   catalog acquisition/verification/cache lifecycle, CLI integration, tests,
   and skeleton documentation update.
3. Implement Engine behavior with tests and keep the online and offline
   semantics above explicit in the user guide.
4. Update the release workflow and GoReleaser configuration to calculate the
   signed catalog digest and embed it in the binaries before publishing the
   matching catalog asset.
5. Update the skeleton's quickstart to pin RoadRunner and run setup, while
   clearly stating that project creation still needs PHP and Composer under
   the currently supported installation path.
6. Release a new Engine version with matching signed catalog assets before
   advertising catalog auto-bootstrap as available.

## Alternatives considered

- **Require users to copy/download the catalog manually:** explicit but
  unnecessarily fragile for the standard official workflow; retain only as
  a development/troubleshooting path.
- **Always use the newest catalog:** rejected because it breaks release
  reproducibility and can exceed the trust/compatibility policy of an older
  Engine binary.
- **Compare `catalog_version` to the Engine tag:** rejected because the
  published `v0.3.2` catalog has `catalog_version` `2026.10.0`; that field is
  the catalog inventory version, not the Engine release. The binary-embedded
  catalog digest provides an exact release binding without changing this
  established meaning.
- **Embed the full catalog in every Engine binary:** rejected because the
  signed release asset already exists, may evolve on its own release cadence,
  and should remain independently inspectable and replaceable through a new
  Engine release.
- **Let Composer install RoadRunner CLI:** rejected for this path because it
  introduces a Composer/PHP bootstrap dependency and does not use the
  Engine's signed, platform-aware toolchain catalog.
