# Release-Bound Toolchain Catalog Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make explicit online toolchain setup fetch, authenticate, cache, and use the signed catalog matching the running Tusk Engine release, while preserving offline guarantees.

**Architecture:** Add a release-version source and an injectable, bounded catalog fetcher in `internal/toolchain`. The CLI derives online/offline flow from validated manifest pins, verifies with the existing official trust anchors and embedded exact-catalog digest, atomically stores only verified catalog bytes, then invokes the provisioner. `SetupService` rechecks the release digest when consuming the cached catalog.

**Tech Stack:** Go standard library (`net/http`, `io`, `os`, `filepath`, `time`), existing Ed25519 catalog verifier and provisioner, Go tests with `httptest` and generated Ed25519 keys, Markdown user docs.

**Spec:** `docs/superpowers/specs/2026-10-10-release-bound-toolchain-catalog-bootstrap-design.md`

## Global Constraints

- Only valid stable Engine release versions may determine catalog URLs; never resolve `latest`.
- The downloaded catalog must verify with the trust anchors embedded in the Engine and its SHA-256 digest must match the digest embedded for that exact release.
- `catalog_version` is independent catalog inventory metadata; never compare it to the Engine version or tag.
- `--offline` must perform no network access; online setup must not silently fall back to a stale project catalog.
- Only explicitly pinned tools without explicit paths may be provisioned; no pins means no catalog request.
- Reject HTTP downgrades, unexpected redirect hosts, oversized/empty/malformed catalogs, invalid signatures, expired catalogs, and digest mismatches.
- Persist catalog bytes atomically only after all official signature, validity, schema, artifact-signature, and digest checks succeed; preserve existing bytes on every failure.
- Never require GitHub credentials, log secrets/catalog contents, mutate global `PATH`, install OS packages, or imply that `composer create-project` no longer needs PHP and Composer.
- Skeleton documentation must explain the PHP/Composer project-creation prerequisite and the explicit RoadRunner pin/setup sequence.

## Review Focus

- A signed but wrong-digest catalog must be rejected before becoming the project cache.
- GitHub redirect behavior must not permit an HTTP downgrade or arbitrary host; pin allowed HTTPS destinations in tests.
- Online failure with an old valid cache must preserve it but must not use it in that online run.
- A system-profile manifest with no version pins must remain a successful no-op with zero catalog requests.
- `--offline` must remain network-silent even for missing catalogs, invalid caches, and missing artifacts.

---

## File map

- Modify `internal/cli/docs.go`: retain one authoritative linker-injected Engine release version and expose the normalized stable version to setup code.
- Create `internal/toolchain/catalog_fetch.go`: release asset URL construction, injectable HTTP retrieval, response limits, redirect validation, and timeout defaults.
- Create `internal/toolchain/catalog_fetch_test.go`: URL/version, transport, redirect, timeout, and response-bound tests.
- Modify `internal/toolchain/catalog.go`: inject/read the official catalog SHA-256 linker value and add a helper that checks bytes against that digest and the existing verifier.
- Modify `internal/toolchain/catalog_official_test.go`: matching/mismatching official digest checks using generated keys.
- Create `internal/toolchain/catalog_cache.go` plus OS-specific rename helpers: atomic verified-byte cache persistence in the `.tusk` directory on Windows, Linux, and macOS.
- Create `internal/toolchain/catalog_cache_test.go`: replacement/preservation/cleanup behavior for verified cache writes.
- Modify `internal/cli/cli.go`: derive pins, resolve online/offline catalog, then call the release-bound provisioner; make fetcher, version, digest, verifier, and provisioner injectable at test boundary.
- Modify `internal/cli/toolchain_test.go`: setup online/offline, no-pin no-op, exit status, and no provisioning on catalog failure.
- Modify `internal/toolchain/setup.go`: export the existing request-selection function as `SetupRequests(manifest Manifest) ([]ToolRequest, error)` and make `SetupService.Run` call it so CLI and provisioner share identical pin/profile semantics.
- Modify `internal/toolchain/setup_test.go`: verify `SetupRequests` preserves current profile, explicit-path, and pin-selection behavior.
- Modify `.goreleaser.yaml` and `.github/workflows/release.yml`: compute the signed catalog's SHA-256 and inject it into the Engine binaries built for that release.
- Modify `internal/toolchain/release_workflow_test.go`: assert digest computation/injection and ordering before GoReleaser publication.
- Modify `release/README.md`: document the digest pin and its release pipeline role.
- Modify `docs/user-guide/toolchain.md`: explain automatic release-bound online catalog retrieval, offline cache constraints, errors, and no implicit installation.
- In the separate `tusk-app` repository, modify the skeleton quickstart README to pin RoadRunner and run setup, with explicit PHP/Composer prerequisite.

## Interfaces

Task 1 produces `toolchain.ReleaseCatalogURL(version string) (string, error)` and `toolchain.NormalizeReleaseVersion(version string) (string, error)`. Input is the GoReleaser version string (`0.3.2`, no leading `v`); URL tag formatting adds `v` exactly once. Development, malformed, and prerelease versions return errors.

Task 2 produces `toolchain.CatalogFetcher` with method `Fetch(ctx context.Context, releaseVersion string) ([]byte, error)` and `toolchain.HTTPReleaseCatalogFetcher{Client *http.Client, MaxBytes int64}`. Its default constructor uses a finite timeout and validates redirects. It does not persist or trust the returned bytes.

Task 3 produces `toolchain.OfficialCatalogDigest() (string, error)`, `toolchain.VerifyOfficialReleaseCatalog(data []byte, verifier CatalogVerifier, expectedDigest string) error`, and `toolchain.WriteCatalogAtomic(root string, data []byte) error`. The verification helper checks the exact SHA-256 and then invokes the existing signature/validity verifier; it does not interpret `catalog_version`. The writer only handles already-verified bytes and writes `.tusk/toolchain.catalog.json` by same-directory temp file and atomic rename.

Task 4 adds an injectable CLI setup dependency bundle containing Engine release version, expected catalog digest, verifier factory, and `CatalogFetcher`. `runToolchainSetupTo` remains the command boundary; it loads/validates the manifest and calls the exported `SetupRequests`; its no-pin path returns success without network or catalog access, while offline loads the existing file only.

## Tasks

### Task 1: Release-version normalization and deterministic asset URL

**Files:**
- Modify: `internal/cli/docs.go`
- Create: `internal/toolchain/catalog_fetch.go`
- Create: `internal/toolchain/catalog_fetch_test.go`

**Interfaces:**
- Produces the URL/version functions defined under Interfaces.
- The CLI release identity remains the existing linker-injected `documentationVersion`; do not introduce a second independent linker variable.

- [x] **Step 1: Write failing normalization and URL tests**

Add `TestNormalizeReleaseVersion` for `0.3.2` -> `0.3.2`, and errors for empty, `dev`, `v0.3.2`, malformed text, and prerelease `0.3.2-rc.1`. Add `TestReleaseCatalogURL` asserting the exact URL `https://github.com/tusk-framework/tusk-engine/releases/download/v0.3.2/toolchain-catalog.json` and rejection of invalid versions.

- [x] **Step 2: Run the focused tests and confirm they fail**

Run: `go test ./internal/toolchain -run 'Test(NormalizeReleaseVersion|ReleaseCatalogURL)$' -count=1`

Expected: FAIL because the release-version functions do not exist.

- [x] **Step 3: Implement stable SemVer validation and URL construction**

Implement `NormalizeReleaseVersion` and `ReleaseCatalogURL` in `internal/toolchain/catalog_fetch.go` using a strict `major.minor.patch` numeric pattern with no leading `v`, prerelease, or build suffix (no new dependency). Keep version syntax strict and build URLs from the fixed official release base, never user-controlled host/path input. Expose the existing CLI-injected version to the setup dependency without duplicating linker state.

- [x] **Step 4: Run focused tests**

Run: `go test ./internal/toolchain -run 'Test(NormalizeReleaseVersion|ReleaseCatalogURL)$' -count=1`

Expected: PASS.

- [x] **Step 5: Commit the version/URL unit** (`ad6fe46`)

```bash
git add internal/cli/docs.go internal/toolchain/catalog_fetch.go internal/toolchain/catalog_fetch_test.go
git commit -m "feat: bind catalog lookup to engine release"
```

### Task 2: Bounded HTTPS catalog fetcher

**Files:**
- Modify: `internal/toolchain/catalog_fetch.go`
- Modify: `internal/toolchain/catalog_fetch_test.go`

**Interfaces:**
- Consumes the exact URL from Task 1.
- Produces `CatalogFetcher.Fetch(ctx, releaseVersion)` and the default HTTP implementation described under Interfaces.

- [x] **Step 1: Write failing HTTP fetch tests**

Add tests for successful 200 response, non-200 status, empty response, response above configured maximum, request cancellation/timeout, redirect to an allowed HTTPS GitHub asset host (`release-assets.githubusercontent.com` or `objects.githubusercontent.com`), HTTP downgrade, unexpected HTTPS host, redirect loop, and no authorization header. Use `httptest.Server` and a custom `http.Client`/round tripper; do not use public network.

- [x] **Step 2: Run focused tests and confirm they fail**

Run: `go test ./internal/toolchain -run 'TestHTTPReleaseCatalogFetcher' -count=1`

Expected: FAIL because fetcher types and behavior do not exist.

- [x] **Step 3: Implement retrieval with limits and redirect policy**

Implement context-aware GET, a finite default timeout (30 seconds), explicit maximum catalog bytes (1 MiB), status validation, bounded reads (limit + 1), empty-body rejection, and strict HTTPS redirect-host validation allowing only the initial `github.com` URL and the two release asset hosts named above. Keep client injection available; do not add credentials or persist data here.

- [x] **Step 4: Run focused tests and race check**

Run: `go test ./internal/toolchain -run 'TestHTTPReleaseCatalogFetcher' -count=1`

Expected: PASS. Then run `go test -race ./internal/toolchain` when the local Go toolchain supports it; otherwise record that CI must run the race check.

- [x] **Step 5: Commit the fetcher** (`75353d7`)

```bash
git add internal/toolchain/catalog_fetch.go internal/toolchain/catalog_fetch_test.go
git commit -m "feat: fetch release catalogs over bounded https"
```

### Task 3: Official release digest verification and atomic catalog cache

**Files:**
- Modify: `internal/toolchain/catalog.go`
- Modify: `internal/toolchain/catalog_official_test.go`
- Create: `internal/toolchain/catalog_cache.go`
- Create: `internal/toolchain/catalog_cache_other.go` with `//go:build !windows`
- Create: `internal/toolchain/catalog_cache_windows.go`
- Create: `internal/toolchain/catalog_cache_test.go`

**Interfaces:**
- Consumes `CatalogVerifier.Verify` and standard SHA-256 normalization.
- Produces `OfficialCatalogDigest()`, `VerifyOfficialReleaseCatalog(data, verifier, expectedDigest)`, and `WriteCatalogAtomic(root, data)`.

- [x] **Step 1: Write failing release-binding verification tests**

Add `TestVerifyOfficialReleaseCatalogRequiresMatchingDigest` using generated Ed25519 keys and signed payloads: bytes with the correct expected SHA-256 and a valid signature pass even when `catalog_version` is `2026.10.0` and the Engine version is `0.3.2`; wrong/malformed/empty expected digests and modified bytes fail. Confirm generic `CatalogVerifier.Verify` remains unchanged.

- [x] **Step 2: Run focused tests and confirm the helper is missing**

Run: `go test ./internal/toolchain -run 'TestVerifyOfficialReleaseCatalogRequiresMatchingDigest$' -count=1`

Expected: FAIL because the official release-bound helper does not exist.

- [x] **Step 3: Implement official release-bound verification** (`c99049c`; `SetupService.Run` now requires and rechecks the expected digest)

Add the `officialCatalogSHA256` linker variable and `OfficialCatalogDigest()` that rejects an absent or malformed release digest. `VerifyOfficialReleaseCatalog` validates/normalizes the expected SHA-256, compares it with the supplied bytes, then invokes the configured verifier. Do not compare `catalog_version` to Engine SemVer, tighten generic verifier semantics, or weaken embedded-anchor/validity checks.

- [x] **Step 4: Write failing atomic cache tests**

Add tests for creating `.tusk/toolchain.catalog.json`, replacing an existing file, preserving existing bytes when temp creation/write/rename fails, leaving no temp files after failure, and not accepting an empty byte slice. Inject only the final replace operation if needed to deterministically test rename failure; avoid a broad filesystem abstraction.

- [x] **Step 5: Implement same-directory atomic persistence**

Implement `WriteCatalogAtomic(root, data)` with `.tusk` directory creation, a restrictive same-directory temporary file, full write, sync, close, and an OS-specific atomic replace helper. On Unix use same-directory `os.Rename`; on Windows use `golang.org/x/sys/windows.MoveFileEx` with replacement and write-through flags. Never delete the old file before the replacement is ready. Remove temporary files on all failures.

- [x] **Step 6: Run catalog-focused tests**

Run: `go test ./internal/toolchain -run 'Test(VerifyOfficialReleaseCatalog|OfficialCatalogDigest|WriteCatalogAtomic)' -count=1`

Expected: PASS; all existing `internal/toolchain` tests continue to pass.

- [x] **Step 7: Commit verification and cache unit** (`212bf6a` and `c99049c`, including the approved design correction)

```bash
git add internal/toolchain/catalog.go internal/toolchain/catalog_official_test.go internal/toolchain/catalog_cache.go internal/toolchain/catalog_cache_other.go internal/toolchain/catalog_cache_windows.go internal/toolchain/catalog_cache_test.go
git commit -m "feat: verify and cache release-bound catalogs"
```

### Task 4: CLI online/offline bootstrap integration

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/toolchain_test.go`
- Modify: `internal/toolchain/setup.go` to export `SetupRequests(manifest Manifest) ([]ToolRequest, error)` and reuse it from `SetupService.Run`.

**Interfaces:**
- Consumes Tasks 1-3.
- `SetupRequests(manifest Manifest)` centralizes existing `setupRequests` behavior; both `SetupService.Run` and CLI integration call the same function.
- Test dependencies permit deterministic injected fetcher, verifier, and release version.

- [x] **Step 1: Add CLI flow tests** (focused tests cover no-pin no-op, online version/digest/cache ordering, offline no-fetch and digest rejection, and online failure preservation; remaining negative cases are covered by lower-level verifier/fetcher tests)

Add tests asserting: online setup with RoadRunner pin fetches once, verifies digest/signature and stores before provisioning; no requests is a successful no-op with zero fetch/provision calls; offline setup never invokes fetch and uses only the existing catalog; missing trust anchors, missing embedded digest, invalid signature, expired catalog, malformed response, digest mismatch, HTTP failure, and fetch timeout prevent provisioning; offline missing/invalid/wrong-digest catalog returns an actionable failure without fetching; failed online refresh preserves existing catalog bytes and does not pass the stale file to provisioner; unsupported dev release returns actionable error; setup failure yields non-zero exit code. Add `TestSetupRequests` cases for each supported profile, explicit paths, no manifest, and CI profile missing pins so the CLI's early inspection exactly matches `SetupService.Run`.

- [x] **Step 2: Run CLI tests and confirm they fail** (initial focused run exposed stale-cache assumptions and version test fixture mismatch; corrected and rerun)

Run: `go test ./internal/cli ./internal/toolchain -run 'Test(ToolchainSetup|SetupRequests)' -count=1`

Expected: FAIL because setup currently requires a preexisting catalog and does not acquire one.

- [x] **Step 3: Implement manifest request inspection at CLI boundary**

Load and validate `.tusk/toolchain.json`, then call `toolchain.SetupRequests`. If no requests exist, report a successful no-op and do not initialize HTTP or verifier dependencies. Preserve current validation errors, including CI-profile unpinned-tool errors.

- [x] **Step 4: Implement online/offline catalog resolution**

For online pinned requests, call the injected fetcher with the exact Engine release version; check bytes against the injected/binary-embedded digest and official verifier before `WriteCatalogAtomic`; only then invoke existing `provisionToolchain`. For offline requests, perform no fetch and check the existing cache against the same embedded digest and trust anchors before provisioning. Do not compare `catalog_version` to Engine version. On online failure, return without using the preexisting catalog. Keep setup command and flags unchanged. Refactor existing unexported `setupRequests` to exported `SetupRequests` and update `SetupService.Run` to call it, with no intended behavior change.

- [x] **Step 5: Run CLI and package tests** (`go test ./internal/toolchain ./internal/cli -count=1` → PASS)

Run: `go test ./internal/cli ./internal/toolchain -count=1`

Expected: PASS, including existing tests and all new online/offline behavior cases.

- [x] **Step 6: Commit CLI integration** (consolidated into the implementation branch after all tasks)

```bash
git add internal/cli/cli.go internal/cli/toolchain_test.go internal/toolchain/setup.go internal/toolchain/setup_test.go
git commit -m "feat: bootstrap signed catalog during toolchain setup"
```

### Task 5: Pin catalog digest in release binaries

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/release.yml`
- Modify: `internal/toolchain/release_workflow_test.go`
- Modify: `release/README.md`

**Interfaces:**
- Consumes the signed `release/toolchain-catalog.json` produced by the existing release step.
- Injects `internal/toolchain.officialCatalogSHA256` into every release binary from `TUSK_CATALOG_SHA256`.

- [x] **Step 1: Add failing release workflow and GoReleaser assertions** (`TestReleaseWorkflowBindsSignedCatalogDigestIntoBinaries` initially failed because workflow/linker injection was absent)

Extend `TestReleaseWorkflowRequiresCatalogSigningAndAttestationSteps` to require a digest step after `tusk-catalog sign`/`verify` and before `Run GoReleaser`, with SHA-256 computed from `release/toolchain-catalog.json` and exported as `TUSK_CATALOG_SHA256`. Extend `TestGoReleaserPublishesCatalogAssetsAndInjectsPublicAnchors` to require `officialCatalogSHA256` and `TUSK_CATALOG_SHA256`.

- [x] **Step 2: Run focused release tests and confirm they fail**

Run: `go test ./internal/toolchain -run 'Test(ReleaseWorkflowRequiresCatalogSigningAndAttestationSteps|GoReleaserPublishesCatalogAssetsAndInjectsPublicAnchors)$' -count=1`

Expected: FAIL because the digest computation and linker injection are absent.

- [x] **Step 3: Compute the exact signed catalog digest before GoReleaser**

Add a bash workflow step after catalog signing/verification and before GoReleaser. Compute `sha256sum release/toolchain-catalog.json`, validate the 64-character lowercase hex digest, and append `TUSK_CATALOG_SHA256=<digest>` to `$GITHUB_ENV`. This step must consume the final signed bytes, not the payload or provenance.

- [x] **Step 4: Inject the digest into release binaries**

Add GoReleaser linker flag `-X github.com/tusk-framework/tusk-engine/internal/toolchain.officialCatalogSHA256={{ .Env.TUSK_CATALOG_SHA256 }}` alongside the existing trust-anchor linker flag. Update `release/README.md` to state that the release job pins the exact signed catalog digest in all binaries and `catalog_version` is independent metadata.

- [x] **Step 5: Run release workflow tests** (existing signing, catalog matrix, GoReleaser, and release-doc checks plus the new digest-binding test all pass)

Run: `go test ./internal/toolchain -run 'Test(ReleaseWorkflowRequiresCatalogSigningAndAttestationSteps|GoReleaserPublishesCatalogAssetsAndInjectsPublicAnchors|ReleaseDocumentationCoversSecretHandlingRotationAndRevocation)$' -count=1`

Expected: PASS.

- [x] **Step 6: Commit release binding** (consolidated into the implementation branch after all tasks)

```bash
git add .goreleaser.yaml .github/workflows/release.yml internal/toolchain/release_workflow_test.go release/README.md
git commit -m "build: pin catalog digest in release binaries"
```

### Task 6: Engine user guide and full verification

**Files:**
- Modify: `docs/user-guide/toolchain.md`
- Modify: `internal/cli/cli_test.go` or `internal/cli/toolchain_test.go` only if final message assertions need a dedicated test.

**Interfaces:**
- Documents the behavior implemented in Task 4; no new runtime interfaces.

- [x] **Step 1: Update setup guide**

Document the explicit RoadRunner pin + online setup example, release-matched signed catalog retrieval, no-pins no-network behavior, offline cache requirement, actionable development-build limitation, and that `start`/`doctor` remain non-provisioning. State that catalog bootstrap does not remove PHP/Composer prerequisites for project creation.

- [x] **Step 2: Verify documentation links and CLI examples** (`go test ./internal/cli -run 'TestRunDocs' -count=1` → PASS)

Run: `go test ./internal/cli -run 'TestRunDocs' -count=1`

Expected: PASS; examples use the existing CLI syntax and do not imply unsupported PHP bundle coverage.

- [x] **Step 3: Run complete Engine verification** (`go test ./...`, `go test -race ./internal/toolchain ./internal/cli`, `go vet ./...`, `go build ./...`, and `git diff --check` all exit 0. Build emitted a non-fatal permission warning while Go attempted to write outside-workspace module stat cache.)

Run: `go test ./...`

Expected: PASS.

Run: `go vet ./...`

Expected: PASS.

Run: `go build ./...`

Expected: PASS.

Run: `git diff --check`

Expected: no whitespace errors.

- [ ] **Step 4: Commit docs and final tests** (consolidated into implementation branch after final verification/review)

```bash
git add docs/user-guide/toolchain.md internal/cli
git commit -m "docs: explain automatic toolchain catalog setup"
```

### Task 7: Update official skeleton quickstart in a coordinated app PR

**Repository:** `tusk-app` (separate worktree and PR, based on its synchronized `origin/main`).

**Files:**
- Modify: the current skeleton `README.md` setup section (confirm exact path at execution time).

**Interfaces:**
- Depends on the Engine implementation from Tasks 1-6 being merged and released with a signed catalog asset matching that Engine release.
- Does not alter Composer manifests or bundle `.tusk/` generated cache state.

- [ ] **Step 1: Add the new quickstart sequence to README tests/fixtures if present**

Assert that the documented steps include `tusk toolchain pin roadrunner@<supported-version>` followed by `tusk setup --toolchain`, and state PHP/Composer are still needed for `composer create-project` under current support.

- [ ] **Step 2: Update README with the released Engine workflow**

Use only an actual RoadRunner version supported by the catalog released with the minimum required Engine version. Explain that the catalog is fetched and verified automatically by Engine and is not committed to the skeleton.

- [ ] **Step 3: Verify and open the separate app PR**

Run the app repository's relevant documentation checks and `git diff --check`; open a PR referencing the Engine release/PR. Do not advertise the sequence until the Engine version it requires is published.

## Final integration checks

- [x] Confirm all tests in the spec's validation strategy map to Tasks 1-6.
- [x] Ensure generic `CatalogVerifier.Verify` remains unchanged while `SetupService` requires the official release digest for provisioning.
- [x] Verify no online path uses an old catalog after a refresh failure, and no offline path initializes network access.
- [x] Run all local Engine verification and request a fresh code review; reviewer found no Critical/Important issues after the required digest check was made mandatory in `SetupService`.
- [ ] Update draft PR #42 with the verified implementation; merge only when required CI checks are green and user workflow approves.
- [ ] After the Engine release, complete the `tusk-app` quickstart PR and verify its links and versioned commands (deferred until a matching Engine release exists).
