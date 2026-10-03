# Tusk Engine Toolchain Provisioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a tested, explicit, offline-capable provisioning core for PHP, Composer, and RoadRunner without mutating global environment state or publishing unverified executables.

**Architecture:** Extend `internal/toolchain` with signed catalog models, bounded downloading, content-addressed cache, safe archive installation, and independent provisioning results. Keep the current resolver and manifest commands compatible; `setup --toolchain` becomes an explicit orchestration entry point but refuses to proceed when no trusted catalog is available. Official platform catalogs are a separate release task after this core is verified.

**Tech Stack:** Go 1.23 standard library (`crypto/ed25519`, `crypto/sha256`, `archive/zip`, `archive/tar`, `compress/gzip`, `net/http`, `os`), existing CLI/config packages, table-driven Go tests, local `httptest` fixtures only.

**Spec:** `docs/superpowers/specs/2026-10-03-toolchain-provisioning-design.md`

## Global Constraints

- Provisioning is never a side effect of `start`, `dev`, package commands, or ordinary `doctor` execution.
- The Engine does not mutate global `PATH`.
- Existing binaries are never overwritten in place.
- Downloads are accepted only over HTTPS and after digest and Ed25519 signature verification.
- Cache entries are content-addressed by SHA-256 and reused only after verification.
- Offline mode fails clearly when a verified cache entry is unavailable.
- Archive extraction rejects absolute paths and `..` traversal.
- Tests must not depend on the public internet.
- Composer remains the dependency solver and lockfile authority; RoadRunner remains the request/worker runtime.

## Review Focus

- Canonical signed payload changes or JSON field-order assumptions: owned by Task 1 tests for stable signing and rejection of altered metadata.
- Digest-valid but signature-invalid artifacts: owned by Task 1 tests for authenticity before acceptance.
- Oversized or cancelled downloads: owned by Task 2 tests for bounded reads and context cancellation.
- ZIP/TAR.GZ traversal, absolute entries, and missing entrypoints: owned by Task 3 tests for safe extraction and no publication.
- Existing target directories and partial failures: owned by Task 4 tests for atomic publication and independent per-tool results.

---

### Task 1: Define artifact and signed catalog contracts

**Files:**
- Create: `internal/toolchain/catalog.go`
- Create: `internal/toolchain/catalog_test.go`
- Modify: `internal/toolchain/doctor.go` only if shared tool-name helpers need extraction

**Interfaces:**
- Produces `Artifact`, `CatalogPayload`, `SignedCatalog`, `CatalogVerifier`, and `CatalogVerifier.Verify(data []byte) (CatalogPayload, error)`.
- `Artifact` fields are `Tool ToolName`, `Version string`, `GOOS string`, `GOARCH string`, `URL string`, `SHA256 string`, `Signature string`, `Format string`, and `EntryPoint string`.
- `CatalogPayload` contains `SchemaVersion int` and `Artifacts []Artifact`.
- `SignedCatalog` contains `Payload CatalogPayload`, `KeyID string`, and base64 `Signature string`.
- `CatalogVerifier` contains `PublicKeys map[string]ed25519.PublicKey` and rejects unknown key IDs, malformed base64, unsupported schema versions, non-HTTPS URLs, and empty artifact identity fields.

- [ ] **Step 1: Write failing tests** for canonical payload verification, altered payload rejection, unknown key ID, malformed signature, invalid URL, and unsupported schema version.
- [ ] **Step 2: Run the focused tests** with `go test ./internal/toolchain -run 'TestCatalog' -count=1`; confirm failure because the catalog types/verifier do not exist.
- [ ] **Step 3: Implement the catalog types and verifier** using deterministic JSON marshaling of the typed payload and `ed25519.Verify`; validate artifact fields before signature acceptance.
- [ ] **Step 4: Run the focused tests** and confirm all catalog tests pass.
- [ ] **Step 5: Run `gofmt -w internal/toolchain/catalog.go internal/toolchain/catalog_test.go` and commit** with `feat(engine): verify signed toolchain catalogs`.

### Task 2: Add bounded downloader and content-addressed cache

**Files:**
- Create: `internal/toolchain/download.go`
- Create: `internal/toolchain/cache.go`
- Create: `internal/toolchain/download_test.go`
- Create: `internal/toolchain/cache_test.go`

**Interfaces:**
- Produces `Downloader` with `Download(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error)`.
- Produces `HTTPDownloader` with `Client *http.Client` and `Timeout time.Duration`; it allows HTTPS only, rejects non-2xx responses, enforces `maxBytes`, and honors context cancellation.
- Produces `ArtifactCache` with `Root string` and methods `Put(expectedSHA256 string, data []byte) (string, error)` and `Get(expectedSHA256 string) ([]byte, error)`.
- `ArtifactCache.Put` writes a temporary file, verifies the exact lowercase SHA-256 digest, atomically renames to `Root/artifacts/<digest>`, and never replaces a different existing digest.
- `ArtifactCache.Get` returns an error for missing or digest-invalid entries.

- [ ] **Step 1: Write failing tests** for HTTPS enforcement, non-2xx response, maximum byte limit, cancellation, cache hit, cache miss, digest mismatch, and atomic cache publication using `httptest.NewTLSServer` for the allowed HTTPS path and temporary directories.
- [ ] **Step 2: Run `go test ./internal/toolchain -run 'Test(HTTPDownloader|ArtifactCache)' -count=1`** and confirm the expected missing-type failures.
- [ ] **Step 3: Implement the downloader** with bounded `io.LimitReader`, response-body cleanup, context-aware HTTP requests, and explicit error messages.
- [ ] **Step 4: Implement the cache** with `sha256.Sum256`, hex-normalized paths, `os.CreateTemp`, `Sync`, close, and `os.Rename`; validate cached content on every read.
- [ ] **Step 5: Run the focused tests** and confirm all downloader/cache tests pass.
- [ ] **Step 6: Run `gofmt` and commit** with `feat(engine): add verified artifact cache`.

### Task 3: Implement safe archive and raw executable installation

**Files:**
- Create: `internal/toolchain/installer.go`
- Create: `internal/toolchain/installer_test.go`

**Interfaces:**
- Produces `Installer` with `Root string`, `GOOS string`, and `GOARCH string`.
- Produces `Install(artifact Artifact, data []byte) (string, error)`.
- Supported formats are `raw`, `zip`, and `tar.gz`; unsupported formats fail before creating a final directory.
- Installation target is `Root/toolchain/<tool>/<version>/<goos>-<goarch>` and the returned path is the validated executable path.
- `Artifact.EntryPoint` is required and must resolve below the temporary extraction root; absolute paths, drive-qualified paths, and cleaned paths containing `..` are rejected.
- Publication occurs only after the expected entrypoint exists, is a regular file, and the final destination does not already exist; Unix executables receive mode `0755`.

- [ ] **Step 1: Write failing tests** for raw installation, ZIP installation, TAR.GZ installation, missing entrypoint, absolute entrypoint, `../` traversal, archive member traversal, unsupported format, and existing destination.
- [ ] **Step 2: Run `go test ./internal/toolchain -run 'TestInstaller' -count=1`** and confirm failure before the installer exists.
- [ ] **Step 3: Implement format-specific staging** in a temporary sibling directory, safe member-path validation, bounded archive extraction, entrypoint validation, and atomic rename into the final destination.
- [ ] **Step 4: Run focused installer tests** and confirm no failed case leaves a published destination.
- [ ] **Step 5: Run `gofmt` and commit** with `feat(engine): safely install toolchain artifacts`.

### Task 4: Orchestrate independent provisioning plans

**Files:**
- Create: `internal/toolchain/provisioner.go`
- Create: `internal/toolchain/provisioner_test.go`
- Modify: `internal/toolchain/doctor.go` to recognize validated installed paths without changing existing precedence

**Interfaces:**
- Produces `ToolRequest { Name ToolName; Version string }`.
- Produces `ProvisionOptions { Offline bool; MaxArtifactBytes int64 }`.
- Produces `ToolProvisionResult { Name ToolName; Version string; Path string; Status string; Error string }`.
- Produces `ProvisionReport { Results []ToolProvisionResult; Ready bool }`.
- Produces `Provisioner { Catalog CatalogPayload; Downloader Downloader; Cache ArtifactCache; Installer Installer; GOOS string; GOARCH string }`.
- Produces `Provision(ctx context.Context, requests []ToolRequest, options ProvisionOptions) ProvisionReport`.
- Each request is resolved independently: explicit verified cache first in offline mode, downloader plus digest verification in online mode, installer publication last. A failed request does not remove or invalidate another successful result.
- After successful installation, the caller receives the path; manifest writing remains a separate explicit operation.

- [ ] **Step 1: Write failing tests** for successful raw/archived provisioning, offline cache hit, offline cache miss, catalog mismatch, digest mismatch, installer failure, and one-tool failure alongside one-tool success.
- [ ] **Step 2: Run `go test ./internal/toolchain -run 'TestProvision' -count=1`** and confirm the expected missing implementation failures.
- [ ] **Step 3: Implement request lookup by exact tool/version/platform**, cache-first behavior, online download, digest checking before cache publication, installer invocation, and per-tool report aggregation.
- [ ] **Step 4: Run focused provisioning tests** and confirm all results and failure isolation behavior.
- [ ] **Step 5: Run the entire `go test ./...` suite** and commit with `feat(engine): orchestrate explicit toolchain provisioning`.

### Task 5: Wire explicit CLI operation and machine-readable diagnostics

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/toolchain/doctor.go`
- Create: `internal/cli/toolchain_test.go`
- Modify: `README.md`

**Interfaces:**
- `tusk setup --toolchain [--offline]` parses flags and invokes the provisioner only when a trusted catalog is available; absent catalog returns a non-success diagnostic explaining how to configure or update the Engine catalog.
- `tusk doctor --json` includes `source`, `version`, `requested_version`, and provisioned/cache status without invoking network access.
- CLI formatting is tested through a command-level helper rather than process termination; existing `Run([]string)` compatibility is preserved.
- No command adds a project-local binary to the process-global `PATH`.

- [ ] **Step 1: Write failing CLI tests** for `doctor --json`, `toolchain list`, `toolchain pin`, `setup --toolchain --offline` without a catalog, and rejection of unknown flags.
- [ ] **Step 2: Run the focused CLI tests** and confirm failure for the new explicit setup behavior.
- [ ] **Step 3: Refactor only the command parsing necessary** to return structured errors to the helper, route the explicit flags, and preserve existing package/script commands.
- [ ] **Step 4: Update README examples** to distinguish diagnosis, pinning, and explicit provisioning; document that no implicit download occurs.
- [ ] **Step 5: Run `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`; commit** with `feat(engine): expose explicit toolchain setup`.

### Task 6: Release-readiness documentation and issue update

**Files:**
- Modify: `docs/superpowers/specs/2026-10-03-toolchain-provisioning-design.md` only for verified implementation status
- Modify: `README.md` if final command output or limitations changed

- [ ] **Step 1: Review the implementation against every acceptance criterion** in the spec and record any intentionally deferred official-catalog/platform packaging work.
- [ ] **Step 2: Run the final verification set**: `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
- [ ] **Step 3: Confirm a clean worktree and update issue #8** with commits, verification output, and the remaining official-catalog release work.
- [ ] **Step 4: Request an independent code review before merge or PR.**
