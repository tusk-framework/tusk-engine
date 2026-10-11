# Release-Bound Toolchain Catalog Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make explicit online toolchain setup fetch, authenticate, cache, and use the signed catalog matching the running Tusk Engine release, while preserving offline guarantees.

**Architecture:** Add a release-version source and an injectable, bounded catalog fetcher in `internal/toolchain`. The CLI derives online/offline flow from validated manifest pins, verifies with the existing official trust anchors, atomically stores only verified catalog bytes, then invokes the existing provisioner. Keep generic `SetupService` usable with caller-supplied catalogs and test keys; enforce official release binding at the CLI bootstrap boundary.

**Tech Stack:** Go standard library (`net/http`, `io`, `os`, `filepath`, `time`), existing Ed25519 catalog verifier and provisioner, Go tests with `httptest` and generated Ed25519 keys, Markdown user docs.

**Spec:** `docs/superpowers/specs/2026-10-10-release-bound-toolchain-catalog-bootstrap-design.md`

## Global Constraints

- Only valid stable Engine release versions may determine catalog URLs; never resolve `latest`.
- The downloaded catalog must verify with the trust anchors embedded in the Engine and its signed `catalog_version` must match the running Engine release.
- `--offline` must perform no network access; online setup must not silently fall back to a stale project catalog.
- Only explicitly pinned tools without explicit paths may be provisioned; no pins means no catalog request.
- Reject HTTP downgrades, unexpected redirect hosts, oversized/empty/malformed catalogs, invalid signatures, expired catalogs, and release-version mismatches.
- Persist catalog bytes atomically only after all official signature, validity, schema, artifact-signature, and release-version checks succeed; preserve existing bytes on every failure.
- Never require GitHub credentials, log secrets/catalog contents, mutate global `PATH`, install OS packages, or imply that `composer create-project` no longer needs PHP and Composer.
- Skeleton documentation must explain the PHP/Composer project-creation prerequisite and the explicit RoadRunner pin/setup sequence.

## Review Focus

- A signed but wrong-release catalog must be rejected before becoming the project cache.
- GitHub redirect behavior must not permit an HTTP downgrade or arbitrary host; pin allowed HTTPS destinations in tests.
- Online failure with an old valid cache must preserve it but must not use it in that online run.
- A system-profile manifest with no version pins must remain a successful no-op with zero catalog requests.
- `--offline` must remain network-silent even for missing catalogs, invalid caches, and missing artifacts.

---

## File map

- Modify `internal/cli/docs.go`: retain one authoritative linker-injected Engine release version and expose the normalized stable version to setup code.
- Create `internal/toolchain/catalog_fetch.go`: release asset URL construction, injectable HTTP retrieval, response limits, redirect validation, and timeout defaults.
- Create `internal/toolchain/catalog_fetch_test.go`: URL/version, transport, redirect, timeout, and response-bound tests.
- Modify `internal/toolchain/catalog.go`: add a narrow official-release verification helper that checks verified payload `catalog_version` against an expected release version without changing generic `CatalogVerifier` behavior.
- Modify `internal/toolchain/catalog_official_test.go`: matching/mismatching official version checks using generated keys.
- Create `internal/toolchain/catalog_cache.go` plus OS-specific rename helpers: atomic verified-byte cache persistence in the `.tusk` directory on Windows, Linux, and macOS.
- Create `internal/toolchain/catalog_cache_test.go`: replacement/preservation/cleanup behavior for verified cache writes.
- Modify `internal/cli/cli.go`: derive pins, resolve online/offline catalog, then call existing provisioner; make fetcher, version, and verifier injectable at test boundary.
- Modify `internal/cli/toolchain_test.go`: setup online/offline, no-pin no-op, exit status, and no provisioning on catalog failure.
- Modify `internal/toolchain/setup.go`: export the existing request-selection function as `SetupRequests(manifest Manifest) ([]ToolRequest, error)` and make `SetupService.Run` call it so CLI and provisioner share identical pin/profile semantics.
- Modify `internal/toolchain/setup_test.go`: verify `SetupRequests` preserves current profile, explicit-path, and pin-selection behavior.
- Modify `docs/user-guide/toolchain.md`: explain automatic release-bound online catalog retrieval, offline cache constraints, errors, and no implicit installation.
- In the separate `tusk-app` repository, modify the skeleton quickstart README to pin RoadRunner and run setup, with explicit PHP/Composer prerequisite.

## Interfaces

Task 1 produces `toolchain.ReleaseCatalogURL(version string) (string, error)` and `toolchain.NormalizeReleaseVersion(version string) (string, error)`. Input is the GoReleaser version string (`0.3.2`, no leading `v`); URL tag formatting adds `v` exactly once. Development, malformed, and prerelease versions return errors.

Task 2 produces `toolchain.CatalogFetcher` with method `Fetch(ctx context.Context, releaseVersion string) ([]byte, error)` and `toolchain.HTTPReleaseCatalogFetcher{Client *http.Client, MaxBytes int64}`. Its default constructor uses a finite timeout and validates redirects. It does not persist or trust the returned bytes.

Task 3 produces `toolchain.VerifyOfficialReleaseCatalog(data []byte, verifier CatalogVerifier, expectedVersion string) error` and `toolchain.WriteCatalogAtomic(root string, data []byte) error`. The verifier helper first calls the existing verifier and then compares the signed catalog version; the writer only handles already-verified bytes and writes `.tusk/toolchain.catalog.json` by same-directory temp file and atomic rename.

Task 4 adds an injectable CLI setup dependency bundle containing release version, verifier factory, and `CatalogFetcher`. `runToolchainSetupTo` remains the command boundary; it loads/validates the manifest and calls the exported `SetupRequests`; its no-pin path returns success without network or catalog access, while offline loads the existing file only.

## Tasks

### Task 1: Release-version normalization and deterministic asset URL

**Files:**
- Modify: `internal/cli/docs.go`
- Create: `internal/toolchain/catalog_fetch.go`
- Create: `internal/toolchain/catalog_fetch_test.go`

**Interfaces:**
- Produces the URL/version functions defined under Interfaces.
- The CLI release identity remains the existing linker-injected `documentationVersion`; do not introduce a second independent linker variable.

- [ ] **Step 1: Write failing normalization and URL tests**

Add `TestNormalizeReleaseVersion` for `0.3.2` -> `0.3.2`, and errors for empty, `dev`, `v0.3.2`, malformed text, and prerelease `0.3.2-rc.1`. Add `TestReleaseCatalogURL` asserting the exact URL `https://github.com/tusk-framework/tusk-engine/releases/download/v0.3.2/toolchain-catalog.json` and rejection of invalid versions.

- [ ] **Step 2: Run the focused tests and confirm they fail**

Run: `go test ./internal/toolchain -run 'Test(NormalizeReleaseVersion|ReleaseCatalogURL)$' -count=1`

Expected: FAIL because the release-version functions do not exist.

- [ ] **Step 3: Implement stable SemVer validation and URL construction**

Implement `NormalizeReleaseVersion` and `ReleaseCatalogURL` in `internal/toolchain/catalog_fetch.go` using a strict `major.minor.patch` numeric pattern with no leading `v`, prerelease, or build suffix (no new dependency). Keep version syntax strict and build URLs from the fixed official release base, never user-controlled host/path input. Expose the existing CLI-injected version to the setup dependency without duplicating linker state.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/toolchain -run 'Test(NormalizeReleaseVersion|ReleaseCatalogURL)$' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the version/URL unit**

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

- [ ] **Step 1: Write failing HTTP fetch tests**

Add tests for successful 200 response, non-200 status, empty response, response above configured maximum, request cancellation/timeout, redirect to an allowed HTTPS GitHub asset host (`release-assets.githubusercontent.com` or `objects.githubusercontent.com`), HTTP downgrade, unexpected HTTPS host, redirect loop, and no authorization header. Use `httptest.Server` and a custom `http.Client`/round tripper; do not use public network.

- [ ] **Step 2: Run focused tests and confirm they fail**

Run: `go test ./internal/toolchain -run 'TestHTTPReleaseCatalogFetcher' -count=1`

Expected: FAIL because fetcher types and behavior do not exist.

- [ ] **Step 3: Implement retrieval with limits and redirect policy**

Implement context-aware GET, a finite default timeout (30 seconds), explicit maximum catalog bytes (1 MiB), status validation, bounded reads (limit + 1), empty-body rejection, and strict HTTPS redirect-host validation allowing only the initial `github.com` URL and the two release asset hosts named above. Keep client injection available; do not add credentials or persist data here.

- [ ] **Step 4: Run focused tests and race check**

Run: `go test ./internal/toolchain -run 'TestHTTPReleaseCatalogFetcher' -count=1`

Expected: PASS. Then run `go test -race ./internal/toolchain` when the local Go toolchain supports it; otherwise record that CI must run the race check.

- [ ] **Step 5: Commit the fetcher**

```bash
git add internal/toolchain/catalog_fetch.go internal/toolchain/catalog_fetch_test.go
git commit -m "feat: fetch release catalogs over bounded https"
```

### Task 3: Official release verification and atomic catalog cache

**Files:**
- Modify: `internal/toolchain/catalog.go`
- Modify: `internal/toolchain/catalog_official_test.go`
- Create: `internal/toolchain/catalog_cache.go`
- Create: `internal/toolchain/catalog_cache_other.go` with `//go:build !windows`
- Create: `internal/toolchain/catalog_cache_windows.go`
- Create: `internal/toolchain/catalog_cache_test.go`

**Interfaces:**
- Consumes `CatalogVerifier.Verify` and version normalization from Task 1.
- Produces `VerifyOfficialReleaseCatalog(data, verifier, expectedVersion)` and `WriteCatalogAtomic(root, data)`.

- [ ] **Step 1: Write failing release-version verification tests**

Add `TestVerifyOfficialReleaseCatalogRequiresMatchingVersion` using generated Ed25519 keys and signed payloads: a valid, unexpired matching `catalog_version` passes; wrong and empty versions fail. Confirm generic `CatalogVerifier.Verify` remains usable by existing tests whose fixture version is `test`.

- [ ] **Step 2: Run focused tests and confirm the helper is missing**

Run: `go test ./internal/toolchain -run 'TestVerifyOfficialReleaseCatalogRequiresMatchingVersion$' -count=1`

Expected: FAIL because the official release-bound helper does not exist.

- [ ] **Step 3: Implement official release-bound verification**

Implement the helper by invoking the configured verifier first, then require exact equality between the normalized expected release and signed `CatalogVersion`. Do not tighten generic verifier semantics or weaken embedded-anchor/validity checks.

- [ ] **Step 4: Write failing atomic cache tests**

Add tests for creating `.tusk/toolchain.catalog.json`, replacing an existing file, preserving existing bytes when temp creation/write/rename fails, leaving no temp files after failure, and not accepting an empty byte slice. Inject only the final replace operation if needed to deterministically test rename failure; avoid a broad filesystem abstraction.

- [ ] **Step 5: Implement same-directory atomic persistence**

Implement `WriteCatalogAtomic(root, data)` with `.tusk` directory creation, a restrictive same-directory temporary file, full write, sync, close, and an OS-specific atomic replace helper. On Unix use same-directory `os.Rename`; on Windows use `golang.org/x/sys/windows.MoveFileEx` with replacement and write-through flags. Never delete the old file before the replacement is ready. Remove temporary files on all failures.

- [ ] **Step 6: Run catalog-focused tests**

Run: `go test ./internal/toolchain -run 'Test(VerifyOfficialReleaseCatalog|WriteCatalogAtomic)' -count=1`

Expected: PASS; all existing `internal/toolchain` tests continue to pass.

- [ ] **Step 7: Commit verification and cache unit**

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

- [ ] **Step 1: Add failing CLI flow tests**

Add tests asserting: online setup with RoadRunner pin fetches once, verifies and stores before provisioning; no requests is a successful no-op with zero fetch/provision calls; offline setup never invokes fetch and uses only the existing catalog; missing trust anchors, invalid signature, expired catalog, malformed response, version mismatch, HTTP failure, and fetch timeout prevent provisioning; offline missing/invalid/wrong-version catalog returns an actionable failure without fetching; failed online refresh preserves existing catalog bytes and does not pass the stale file to provisioner; unsupported dev release returns actionable error; setup failure yields non-zero exit code. Add `TestSetupRequests` cases for each supported profile, explicit paths, no manifest, and CI profile missing pins so the CLI's early inspection exactly matches `SetupService.Run`.

- [ ] **Step 2: Run CLI tests and confirm they fail**

Run: `go test ./internal/cli ./internal/toolchain -run 'Test(ToolchainSetup|SetupRequests)' -count=1`

Expected: FAIL because setup currently requires a preexisting catalog and does not acquire one.

- [ ] **Step 3: Implement manifest request inspection at CLI boundary**

Load and validate `.tusk/toolchain.json`, then call `toolchain.SetupRequests`. If no requests exist, report a successful no-op and do not initialize HTTP or verifier dependencies. Preserve current validation errors, including CI-profile unpinned-tool errors.

- [ ] **Step 4: Implement online/offline catalog resolution**

For online pinned requests, call the injected fetcher with the exact release version; call official release-bound verification before `WriteCatalogAtomic`; only then invoke existing `provisionToolchain`. For offline requests, perform no fetch and verify the existing cache against embedded anchors and matching release version before provisioning. On online failure, return without using the preexisting catalog. Keep setup command and flags unchanged. Refactor existing unexported `setupRequests` to exported `SetupRequests` and update `SetupService.Run` to call it, with no intended behavior change.

- [ ] **Step 5: Run CLI and package tests**

Run: `go test ./internal/cli ./internal/toolchain -count=1`

Expected: PASS, including existing tests and all new online/offline behavior cases.

- [ ] **Step 6: Commit CLI integration**

```bash
git add internal/cli/cli.go internal/cli/toolchain_test.go internal/toolchain/setup.go internal/toolchain/setup_test.go
git commit -m "feat: bootstrap signed catalog during toolchain setup"
```

### Task 5: Engine user guide and full verification

**Files:**
- Modify: `docs/user-guide/toolchain.md`
- Modify: `internal/cli/cli_test.go` or `internal/cli/toolchain_test.go` only if final message assertions need a dedicated test.

**Interfaces:**
- Documents the behavior implemented in Task 4; no new runtime interfaces.

- [ ] **Step 1: Update setup guide**

Document the explicit RoadRunner pin + online setup example, release-matched signed catalog retrieval, no-pins no-network behavior, offline cache requirement, actionable development-build limitation, and that `start`/`doctor` remain non-provisioning. State that catalog bootstrap does not remove PHP/Composer prerequisites for project creation.

- [ ] **Step 2: Verify documentation links and CLI examples**

Run: `go test ./internal/cli -run 'TestRunDocs' -count=1`

Expected: PASS; examples use the existing CLI syntax and do not imply unsupported PHP bundle coverage.

- [ ] **Step 3: Run complete Engine verification**

Run: `go test ./...`

Expected: PASS.

Run: `go vet ./...`

Expected: PASS.

Run: `go build ./...`

Expected: PASS.

Run: `git diff --check`

Expected: no whitespace errors.

- [ ] **Step 4: Commit docs and final tests**

```bash
git add docs/user-guide/toolchain.md internal/cli
git commit -m "docs: explain automatic toolchain catalog setup"
```

### Task 6: Update official skeleton quickstart in a coordinated app PR

**Repository:** `tusk-app` (separate worktree and PR, based on its synchronized `origin/main`).

**Files:**
- Modify: the current skeleton `README.md` setup section (confirm exact path at execution time).

**Interfaces:**
- Depends on the Engine implementation from Tasks 1-5 being merged and released with a signed catalog asset matching that Engine release.
- Does not alter Composer manifests or bundle `.tusk/` generated cache state.

- [ ] **Step 1: Add the new quickstart sequence to README tests/fixtures if present**

Assert that the documented steps include `tusk toolchain pin roadrunner@<supported-version>` followed by `tusk setup --toolchain`, and state PHP/Composer are still needed for `composer create-project` under current support.

- [ ] **Step 2: Update README with the released Engine workflow**

Use only an actual RoadRunner version supported by the catalog released with the minimum required Engine version. Explain that the catalog is fetched and verified automatically by Engine and is not committed to the skeleton.

- [ ] **Step 3: Verify and open the separate app PR**

Run the app repository's relevant documentation checks and `git diff --check`; open a PR referencing the Engine release/PR. Do not advertise the sequence until the Engine version it requires is published.

## Final integration checks

- [ ] Confirm all tests in the spec's validation strategy map to Tasks 1-5.
- [ ] Ensure generic test-key catalog flows and release catalog behavior remain separate.
- [ ] Verify no online path uses an old catalog after a refresh failure, and no offline path initializes network access.
- [ ] Run all Engine CI checks and open/refresh the implementation PR; merge only when required checks are green and user workflow approves.
- [ ] After the Engine release, complete the `tusk-app` quickstart PR and verify its links and versioned commands.
