# Portable PHP Runtime Bundles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Publish authenticated, relocatable PHP CLI runtime bundles with a reviewed Tusk extension profile for every advertised platform.

**Architecture:** Build PHP from a pinned source-build toolchain and Tusk-owned recipe; do not consume upstream nightly PHP binaries. Use static-php-cli v2 as the first build candidate, gated by a cross-platform proof, then publish immutable bundles and signed metadata through the existing catalog and release process. The upstream project describes Linux, macOS, and Windows support and a configurable extension list, but its build and redistribution output must be independently verified by Tusk: https://github.com/crazywhalecc/static-php-cli/tree/main.

**Tech Stack:** Go 1.23, static-php-cli (pinned source revision), GitHub Actions, signed toolchain catalog, PHP CLI, Composer platform packages.

**Spec:** `docs/superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md`

## Global Constraints

- Prefer project-local, pinned, verified toolchains; never mutate the global `PATH` or replace system-managed tools.
- Keep acquisition explicit. `tusk start`, `tusk dev`, ordinary diagnostics, and package commands do not perform an implicit network installation.
- Keep one source of truth for dependency requirements and lockfiles.
- Fail with actionable diagnostics when a platform, PHP extension, or native library combination is unsupported.
- Treat downloaded runtimes, extensions, and Composer as executable supply chain inputs: authenticate metadata, pin digests, retain provenance, and publish platform/license information.
- The standard profile must satisfy the requirements declared by the Framework packages and the generated skeleton.
- Do not claim to eliminate operating-system libraries or vendor runtimes unless the bundle actually includes them and the license permits redistribution.

## Review Focus

- Unsupported OS/architecture or libc variant: catalog selection must fail closed with a target-specific error; test exact and near-match targets.
- PHP extension inventory differs from the built executable: release validation must compare `php -m` with the signed profile; test both missing and unexpected modules.
- Extension ABI/module API does not match the PHP build: reject invalid metadata before catalog signing; test mismatched PHP and Zend module API values.
- A supposedly relocatable binary needs an undeclared host library: run it on clean target runners and test `php -v` plus the full module list there.
- Runtime/extension license inventory is incomplete: block publication unless the build emits and validates the license report for the exact artifact.

---

### Task 1: Prove the pinned source-build candidate and extension profile

**Files:**
- Create: `release/php/README.md` — build provenance, supported target policy, and reproducibility procedure.
- Create: `release/php/standard-extensions.txt` — reviewed extension build set, derived from Framework/skeleton requirements plus explicitly supported first-party integrations.
- Create: `release/php/source.lock.json` — immutable upstream source revision and build-tool versions; never use a branch, nightly URL, or mutable tag as the build input.
- Create: `.github/workflows/php-runtime.yml` — build/test matrix for Windows x64, Linux x64/arm64, and macOS x64/arm64.
- Test: `internal/toolchain/php_bundle_workflow_test.go` — static contract checks for pinned inputs, all target entries, no nightly binary download, and license output.

**Interfaces:**
- Consumes: Framework `composer.json` runtime `ext-*` declarations and the approved generated-skeleton requirements.
- Produces: a reproducible archive per supported target plus machine-readable build metadata (PHP version, PHP API, Zend module API, extension names, host prerequisites, licenses, source revision, archive digest).

- [ ] **Step 1: Write failing workflow-contract tests** for all five target tuples, immutable source revision, extension-list presence, and license-report generation.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run TestPHPBundleWorkflow -count=1`; verify failures identify each missing contract.
- [ ] **Step 3: Pin static-php-cli source and build tools** in `release/php/source.lock.json`; write the standard extension set and build recipe. Do not trust or publish upstream prebuilt/nightly PHP binaries.
- [ ] **Step 4: Build and execute the matrix**. Each artifact must run `php -v`, produce its declared modules with `php -m`, and pass the application-requirement profile; record remaining host prerequisites and license inventory.
- [ ] **Step 5: Run** `go test ./internal/toolchain -run TestPHPBundleWorkflow -count=1`; expect the contract tests to pass. Preserve build logs and artifact digests as workflow outputs.
- [ ] **Step 6: Stop and revise the source-build choice** if a target cannot be built reproducibly, required extensions are unavailable, licenses cannot be redistributed, or a clean runner needs undocumented libraries. Do not silently narrow the supported platform matrix.
- [ ] **Step 7: Commit** `build: add reproducible PHP runtime bundle matrix`.

### Task 2: Authenticate PHP runtime metadata in the toolchain catalog

**Files:**
- Modify: `internal/toolchain/catalog.go` — add optional typed PHP runtime metadata to `Artifact` and include it in canonical artifact signing.
- Modify: `internal/toolchain/catalog_test.go` — cover PHP metadata validation and canonical signing.
- Modify: `internal/toolchain/catalog_release_test.go` — require complete metadata on new official PHP artifacts while preserving historical schema-1 verification.
- Modify: `release/toolchain-catalog.payload.json` — add real reviewed bundle URLs, digests, targets, extension inventory, and host prerequisite metadata after Task 1 artifacts exist.
- Test: `internal/toolchain/catalog_test.go`, `internal/toolchain/catalog_release_test.go`.

**Interfaces:**
- Produces: `PHPRuntimeMetadata{Profile, PHPAPI, ZendModuleAPI string; Extensions, HostDependencies, Licenses []string}` as an optional `Artifact.PHP` field. `Licenses` contains SPDX identifiers. Existing artifacts with the field omitted retain their current signature bytes and behavior.

- [ ] **Step 1: Write failing tests** for round-trip/canonical signature inclusion, wrong-tool metadata, duplicate/unsafe extension names, missing API identifiers, and legacy metadata-free artifact verification.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run 'Test(Catalog.*PHP|OfficialCatalog.*PHP)' -count=1`; verify the new cases fail while legacy fixtures still pass.
- [ ] **Step 3: Implement** the typed metadata and strict validation. Ensure artifact signatures cover every metadata field and old schema-1 payload serialization remains byte-compatible when the field is empty.
- [ ] **Step 4: Add real payload entries** only for completed, digest-verified artifacts; do not add placeholders or claim targets without clean-runner evidence.
- [ ] **Step 5: Run** `go test ./internal/toolchain -count=1` and `go run ./cmd/tusk-catalog validate --download --payload release/toolchain-catalog.payload.json`; expect all existing signatures and new artifact metadata to validate.
- [ ] **Step 6: Commit** `feat(toolchain): authenticate PHP runtime profile metadata`.

### Task 3: Publish and attest immutable runtime bundle releases

**Files:**
- Modify: `.github/workflows/php-runtime.yml` — attach bundle archives, license reports, build logs, and provenance to a versioned runtime release.
- Modify: `.github/workflows/release.yml` — validate the reviewed payload and publish the signed catalog only after referenced runtime assets are available.
- Modify: `release/README.md` — document runtime artifact versioning, catalog update ordering, license review, and rollback/revocation.
- Test: `internal/toolchain/release_workflow_test.go` — require validation-before-signing and prevent publishing PHP metadata without immutable artifacts.

**Interfaces:**
- Consumes: Task 1 bundle archive and metadata outputs; Task 2 signed-catalog schema.
- Produces: versioned HTTPS release URLs and checksums accepted by the official catalog verifier.

- [ ] **Step 1: Add failing workflow contract tests** for build-before-catalog ordering, missing-asset failure, and license/provenance publication.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run 'Test(ReleaseWorkflow|OfficialCatalog)' -count=1`; verify the new assertions fail.
- [ ] **Step 3: Wire the runtime asset release** so archives are immutable and independently downloadable before a catalog payload references them; retain the existing fail-closed signing and trust-anchor behavior.
- [ ] **Step 4: Run** `go test ./internal/toolchain -count=1`, validate a real downloaded catalog, and run each advertised bundle in the clean-host matrix.
- [ ] **Step 5: Commit** `ci: publish attested PHP runtime bundles`.

## Execution Order

Task 1 is a release gate. Task 2 consumes its actual outputs; Task 3 must not publish until both pass. The extension-diagnostics plan consumes `PHPRuntimeMetadata` only after Task 2 is merged.
