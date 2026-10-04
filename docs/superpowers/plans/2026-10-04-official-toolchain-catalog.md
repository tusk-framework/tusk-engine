# Official Signed Toolchain Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fail-closed official catalog trust path, deterministic signer/provenance tool, release workflow wiring, and rotation/revocation documentation without changing provisioning orchestration.

**Architecture:** Extend the existing signed catalog model with release metadata and a build-injected public-key set. Add a small `cmd/tusk-catalog` wrapper around tested internal signing/provenance functions, then configure GoReleaser and GitHub Actions to require release inputs and publish the signed catalog as an asset.

**Tech Stack:** Go 1.23 standard library (`crypto/ed25519`, `crypto/sha256`, `encoding/base64`, `encoding/json`, `time`), GitHub Actions, GoReleaser, table-driven Go tests.

**Spec:** `docs/superpowers/specs/2026-10-04-official-toolchain-catalog-design.md`

## Global Constraints

- No private key is committed, generated into the repository, or written to a release workspace by the workflow.
- Unsigned or unknown-key catalogs are rejected; missing trust-anchor configuration fails closed.
- Existing provisioner orchestration and its injected test contracts remain unchanged.
- Release inputs are digest-pinned and HTTPS-only through the existing catalog validator.
- All tests and release validation run without public internet access.

## Review Focus

- Missing or malformed build-injected anchors: `OfficialCatalogVerifier` must fail closed.
- Local unsigned or unknown-key catalog: `LoadOfficialCatalog` must reject before returning payload data.
- Expired/not-yet-valid catalog: official verification must reject its validity window.
- Signing-key/anchor mismatch: signer must refuse to create a release catalog.
- Non-deterministic provenance or output: repeated signing/provenance generation with the same inputs must match byte-for-byte.

---

### Task 1: Add official trust-anchor and catalog policy contracts

**Files:**
- Modify: `internal/toolchain/catalog.go`
- Create: `internal/toolchain/trust_anchor.go`
- Create: `internal/toolchain/catalog_official_test.go`

**Interfaces:**
- Add `CatalogPayload` fields `CatalogVersion`, `IssuedAt`, and `ExpiresAt`.
- Add `OfficialCatalogVerifier() (CatalogVerifier, error)` and `LoadOfficialCatalog(path string) (CatalogPayload, error)`.
- Keep `CatalogVerifier.Verify` compatible for existing injected tests; an official verifier enables freshness enforcement.

- [ ] **Step 1: Write failing tests** for missing anchor, malformed anchor, unsigned local catalog, unknown/revoked key, valid signed catalog, expired catalog, and not-yet-valid catalog.
- [ ] **Step 2: Run `go test ./internal/toolchain -run 'Test(Official|LoadOfficial)' -count=1`** and verify the failures are caused by missing official trust-path behavior.
- [ ] **Step 3: Implement build-injected anchor decoding, freshness validation, and authenticated file loading** without changing provisioner orchestration.
- [ ] **Step 4: Run the focused tests and the existing catalog tests**; confirm both official and injected test paths pass.
- [ ] **Step 5: Commit** with `feat(engine): add official catalog trust anchor`.

### Task 2: Add deterministic catalog signer and provenance command

**Files:**
- Create: `internal/toolchain/catalog_release.go`
- Create: `internal/toolchain/catalog_release_test.go`
- Create: `cmd/tusk-catalog/main.go`

**Interfaces:**
- Add tested signing from `CatalogPayload`, key ID, and in-memory `ed25519.PrivateKey`.
- Add deterministic provenance generation from payload bytes, signed catalog bytes, and key ID.
- The command reads `TUSK_CATALOG_SIGNING_KEY_B64`, `TUSK_CATALOG_KEY_ID`, and `TUSK_CATALOG_TRUST_ANCHORS_B64`; it has no private-key file option.

- [ ] **Step 1: Write failing tests** for deterministic signed output, artifact signatures, malformed private key, signing-key/anchor mismatch, and deterministic provenance.
- [ ] **Step 2: Run `go test ./internal/toolchain -run 'Test(CatalogSigning|CatalogProvenance)' -count=1`** and confirm expected missing-function failures.
- [ ] **Step 3: Implement canonical signing, anchor matching, atomic output, and provenance generation using only standard library code.**
- [ ] **Step 4: Add the thin command wrapper and test its build path** with `go test ./cmd/tusk-catalog` and `go build ./cmd/tusk-catalog`.
- [ ] **Step 5: Commit** with `feat(engine): add reproducible catalog release tooling`.

### Task 3: Wire release assets and operational documentation

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `.github/workflows/release.yml`
- Create: `release/README.md`
- Modify: `README.md`

- [ ] **Step 1: Write documentation/workflow validation checks** as focused static assertions where practical, including required secret/variable names and absence of private-key files.
- [ ] **Step 2: Update GoReleaser** to attach signed catalog and provenance assets and inject the public trust-anchor set through linker flags.
- [ ] **Step 3: Update the workflow** to require release payload/configuration, sign and verify before GoReleaser, and request GitHub build-provenance permissions.
- [ ] **Step 4: Document first-release prerequisites, reproducibility, rotation overlap, revocation, and the fact that payload input is never a trusted runtime catalog.
- [ ] **Step 5: Run the complete verification set and commit** with `ci: release signed toolchain catalog with provenance`.

### Task 4: Final verification and local handoff

- [ ] **Step 1: Review the diff** for private-key material, local unsigned catalog acceptance, provisioner changes, and workflow secret leaks.
- [ ] **Step 2: Run `go test ./...`, `go vet ./...`, `go build ./...`, `go build ./cmd/tusk-catalog`, and `git diff --check`.
- [ ] **Step 3: Confirm the isolated branch status and record all local commit SHAs.**
- [ ] **Step 4: Report release-dependent limitations; do not push, merge, or create a pull request.**
