# Reproducible PHP Toolchain Product Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `tusk-engine` diagnose, resolve, explicitly provision, and run PHP, Composer, and RoadRunner through reproducible profiles without global PATH mutation.

**Architecture:** Extend the existing `internal/toolchain` contracts with manifest profiles, platform-aware policy, and process-local command environments. Add a small product service that consumes the already verified catalog/provisioner and atomically records resulting paths, then make CLI/runtime/package commands use the service. Keep issue #11's trust anchor and release workflow external.

**Tech Stack:** Go 1.23 standard library, existing `internal/toolchain`, `os/exec`, JSON manifest, table-driven tests, no network-dependent tests.

**Spec:** `docs/superpowers/specs/2026-10-04-toolchain-product-design.md`

## Global Constraints

- Provisioning is explicit; doctor, start, and Composer commands never download implicitly.
- Only catalogs accepted by `CatalogVerifier` may reach the provisioner.
- PHP, Composer, and RoadRunner requests/results are independent.
- Composer remains the dependency solver and `composer.lock` authority.
- The parent process PATH is never mutated.
- Existing/user-managed binaries are never overwritten.
- `system`, `project-local`, `docker`, and `ci` profiles are cross-platform policies, not release workflows.
- Issue #11 workflows, trust anchors, catalog publication, and signing keys remain untouched.

## Review Focus

- Legacy manifests without profile/platform fields still resolve exactly as before: Task 1.
- A Windows path and PATH separator must not be interpreted as a Unix path: Task 2.
- One unavailable tool must not hide successful independent resolutions: Task 2.
- A provisioned path must not be persisted before verified installation: Task 3.
- Setup must never turn a present-but-untrusted catalog into an accepted catalog: Task 3.

### Task 1: Manifest profiles and actionable diagnosis

**Files:**
- Modify: `internal/toolchain/doctor.go`
- Modify: `internal/toolchain/doctor_test.go`

**Interfaces:** Add profile/target types, manifest validation, profile policy, and report fields while preserving existing JSON fields and resolver injection. Reject unknown profiles, malformed targets, and unsafe managed paths with errors naming the field and remediation.

- [ ] Write failing tests for legacy manifests, profile round-trip, target defaults, unknown profile, and Windows path normalization.
- [ ] Run `go test ./internal/toolchain -run 'Test(Manifest|Profile|Diagnose)' -count=1` and observe the expected failures.
- [ ] Implement the minimal manifest/profile/policy model and report diagnostics.
- [ ] Run the focused tests and refactor only with green tests.
- [ ] Commit `feat(engine): add toolchain profiles and manifest targets`.

### Task 2: Isolated command environment and independent resolution

**Files:**
- Create: `internal/toolchain/environment.go`
- Create: `internal/toolchain/environment_test.go`
- Modify: `internal/toolchain/doctor.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/toolchain_test.go`

**Interfaces:** Add a process-local environment builder and resolved command representation. It must derive PATH entries for the selected profile/platform, expose per-tool executable paths, and leave `os.Getenv("PATH")` unchanged. Integrate PHP/RoadRunner startup and Composer-backed commands without reimplementing Composer behavior.

- [ ] Write failing tests for PATH isolation on Windows/Linux/macOS, project-local precedence, Composer PHAR command construction, and unchanged parent PATH.
- [ ] Run the focused environment/CLI tests and observe the expected missing API failures.
- [ ] Implement environment construction and wire command creation through injected runner seams so tests do not invoke real PHP.
- [ ] Run focused tests, then `go test ./internal/cli ./internal/toolchain`.
- [ ] Commit `feat(engine): isolate project toolchain execution`.

### Task 3: Explicit setup service and verified manifest persistence

**Files:**
- Create: `internal/toolchain/setup.go`
- Create: `internal/toolchain/setup_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/toolchain_test.go`

**Interfaces:** Add an explicit setup service that accepts a verified `CatalogPayload`, provision options, and manifest; requests only pinned tools, records independent results, and atomically writes returned project-local paths. Add a catalog-loading seam requiring a `CatalogVerifier`, so absent/untrusted catalogs fail before network or installation. Keep current no-catalog behavior and non-zero setup exits.

- [ ] Write failing tests for all three independent requests, partial success, offline cache, absent catalog, untrusted catalog, and atomic manifest persistence.
- [ ] Run `go test ./internal/toolchain ./internal/cli -run 'Test(Setup|ToolchainSetup)' -count=1` and observe red failures.
- [ ] Implement setup orchestration, profile policy, safe relative paths, and actionable aggregated errors.
- [ ] Run focused tests and ensure no manifest is changed on a failed request.
- [ ] Commit `feat(engine): wire verified toolchain setup`.

### Task 4: Product commands, runtime metadata, and documentation

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/toolchain_test.go`
- Modify: `internal/control/metadata.go`
- Modify: `internal/control/metadata_test.go`
- Modify: `README.md`

**Interfaces:** Add `test --provision` handling through the same explicit setup path, expose selected tool versions/profile/target in safe Engine metadata, and document system/project-local/Docker/CI usage plus Windows/Linux/macOS remediation. Do not edit `.github/workflows/release.yml` or add release automation.

- [ ] Write failing tests for `test --provision`, metadata toolchain fields, and actionable human/JSON diagnostics.
- [ ] Run focused tests and observe red failures.
- [ ] Implement the smallest CLI/metadata/documentation changes, preserving existing commands and Composer lockfile semantics.
- [ ] Run `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
- [ ] Commit `feat(engine): complete toolchain product workflow`.

### Task 5: Final verification

- [ ] Re-read the spec and issue acceptance criteria against the diff.
- [ ] Run the full verification set again from the isolated branch.
- [ ] Confirm no release workflow, catalog publication, or trust-anchor file changed.
- [ ] Report commits, files, tests, limitations, and unpushed local branch state.
