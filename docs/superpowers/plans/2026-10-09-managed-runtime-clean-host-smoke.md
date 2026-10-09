# Managed Runtime Clean-Host Smoke Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prove a generated Tusk application can provision PHP, Composer, and RoadRunner locally and serve a request on each advertised platform with no global PHP or Composer installed.

**Architecture:** Extend the existing generated-skeleton smoke flow to use a built Engine binary, project pins, and the official signed catalog. Run each job with a sanitized host `PATH`, provision the toolchain explicitly, install Framework dependencies through the managed Composer/PHP pair, validate real platform requirements, and start the app through RoadRunner. Keep unit tests independent of public network and run this release-level smoke only against immutable published artifacts.

**Tech Stack:** Go 1.23, PowerShell/Bash smoke scripts, GitHub Actions OS/architecture matrix, Tusk Engine, Framework skeleton, signed catalog, RoadRunner, Composer.

**Spec:** `docs/superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md`

## Global Constraints

- The project-local toolchain is pinned, signature/digest verified, and does not mutate host `PATH`.
- No global PHP or Composer may be available to the smoke application.
- Composer remains dependency/lockfile authority; RoadRunner remains runtime owner.
- No runtime setup downloads occur implicitly from `tusk start`; the smoke explicitly invokes setup first.
- Advertise a platform only after its real artifact and clean-host smoke pass.
- CI must not store or expose private catalog signing material; only the release workflow signs official catalogs.

## Review Focus

- A system PHP or Composer leaks into the job through inherited environment: sanitize command lookup and assert `where/which` cannot find them.
- Engine and catalog trust anchors disagree: check signature verification before the first artifact download and stop immediately on failure.
- A target's bundle depends on a host library missing from a fresh runner: test on clean runners and report the declared prerequisite.
- Composer installs dependencies but the selected PHP lacks an extension: require real platform check success before start.
- The app starts with the wrong PHP/RoadRunner executable from PATH: assert reported resolved paths and process command lines are project-local.

---

### Task 1: Make the smoke harness accept a built Engine and sanitized environment

**Files:**
- Modify: `test/skeleton/smoke.ps1` — accept explicit Engine/catalog paths and run all commands from the generated app root.
- Create: `test/skeleton/smoke.sh` — equivalent Bash flow for Linux/macOS with safe cleanup and result reporting.
- Modify: `test/skeleton/README.md` — prerequisites, invocation, evidence, and cleanup behavior.
- Test: `test/skeleton/smoke_contract_test.go` — shell/PowerShell contract checks for sanitized PATH and explicit setup ordering.

**Interfaces:**
- Produces: `smoke.ps1 -EnginePath <path> -CatalogPath <path>` and `smoke.sh --engine <path> --catalog <path>` return non-zero on any setup/install/check/start/request/cleanup failure.
- The harness removes PHP/Composer installation directories from child `PATH` while retaining only OS tools needed by the smoke itself.

- [ ] **Step 1: Write failing contract tests** for required arguments, no system PHP/Composer lookup, explicit `setup --toolchain`, and failure propagation.
- [ ] **Step 2: Run** `go test ./test/skeleton -count=1`; verify missing contracts fail.
- [ ] **Step 3: Implement the harnesses** with project-local temporary app directories, generated fixture/skeleton, deterministic timeout, and safe cleanup.
- [ ] **Step 4: Run** contract tests and a local dry-run that does not download artifacts; expect non-zero for absent catalog/binary with a precise diagnostic.
- [ ] **Step 5: Commit** `test: add clean-host managed runtime smoke harness`.

### Task 2: Exercise the complete managed toolchain lifecycle

**Files:**
- Modify: `test/skeleton/smoke.ps1`, `test/skeleton/smoke.sh` — provision, install, check, start, request, and stop.
- Modify: `test/skeleton/smoke_contract_test.go` — assert lifecycle ordering and no fallback.
- Test: existing `test/skeleton` integration fixtures.

**Interfaces:**
- Consumes: runtime bundles, catalog, extension-aware environment, managed Composer commands, platform check, and offline docs plans.
- Produces: smoke record containing OS/architecture, Engine version, catalog version, selected PHP/Composer/RoadRunner versions and paths, loaded extension list, HTTP status, and shutdown result. Do not include secrets.

- [ ] **Step 1: Write failing lifecycle assertions** for command order, required extensions, request response, and clean shutdown.
- [ ] **Step 2: Run** `go test ./test/skeleton -run TestManagedRuntimeSmoke -count=1`; verify the current harness does not satisfy the contract.
- [ ] **Step 3: Implement** the workflow: `tusk setup --toolchain`, `tusk install`, `tusk check-platform-reqs`, `tusk start`, one HTTP request, graceful stop; assert all resolved PHP/Composer/RoadRunner paths are project-local.
- [ ] **Step 4: Run the harness** against each released platform bundle; fail if it falls back to system tools or if `doctor`/setup reports an unsupported target.
- [ ] **Step 5: Commit** `test: prove Tusk runs without global PHP or Composer`.

### Task 3: Gate platform claims on published-artifact smoke evidence

**Files:**
- Modify: `.github/workflows/skeleton-smoke.yml` — matrix over supported OS/architectures, using only immutable Engine/runtime release assets and a public signed catalog.
- Modify: `release/README.md` — document the required smoke evidence and how to disable a broken target from catalog publication.
- Modify: `internal/toolchain/release_workflow_test.go` — require smoke completion before the release/catalog target is marked supported.

**Interfaces:**
- Consumes: Tasks 1-2; official catalog trust anchors available to the release CI as public material.
- Produces: per-target green check and retained sanitized smoke report; no artifact from an unpassed target may be advertised.

- [ ] **Step 1: Write failing workflow-contract tests** for matrix coverage, clean PATH, published immutable artifacts, and release gating.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run 'Test(ReleaseWorkflow|SkeletonSmoke)' -count=1`; verify new target gates fail.
- [ ] **Step 3: Add the CI matrix** only for platform runners and architecture emulation that can accurately validate the target; use a real native runner for platform-specific runtime behavior.
- [ ] **Step 4: Run** `go test ./...`, `go vet ./...`, and each advertised smoke job; require all target evidence to pass before adding the target to the official payload.
- [ ] **Step 5: Commit** `ci: gate runtime catalog on clean-host smoke tests`.

## Execution Order

This is the final integration plan and depends on the bundle, extension-aware toolchain, and managed-Composer plans. Offline CLI docs may merge earlier but must be included in the final artifact build verification.
