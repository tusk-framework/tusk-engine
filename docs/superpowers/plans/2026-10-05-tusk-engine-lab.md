# Tusk Engine Lab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an external, repeatable local laboratory that validates the Tusk Engine CLI, signed toolchain, Framework bootstrap, RoadRunner runtime, control plane, lifecycle, and failure behavior.

**Architecture:** The lab lives at `C:\Users\wende\Projects\tusk\tusk-engine-lab` and owns all generated application state, downloads, logs, ports, and results. A shared fixture is exercised in `local-source` mode with a locally built Engine and in `published-release` mode with a pinned release binary/catalog. Bash is the WSL/Linux entry point and PowerShell is the Windows entry point; both produce the same result contract.

**Tech Stack:** Bash 5+, PowerShell 7+, PHP 8.2+, Composer, Go, curl, jq, Tusk Framework generator, Tusk Engine, RoadRunner, and Git.

**Spec:** `docs/superpowers/specs/2026-10-05-tusk-engine-lab-design.md`

## Global Constraints

- The project lives outside the Engine and Framework repositories at `C:\Users\wende\Projects\tusk\tusk-engine-lab`.
- Published mode must validate release `v0.3.2` by default and must not use repository working-tree files as a substitute for release assets.
- The fixture must use `bootstrap/app.php` and must not contain a root `worker.php`.
- `.tusk/runtime/worker.php` is Engine-owned generated state and must never be hand-authored by the lab fixture.
- RoadRunner owns HTTP, Goridge, PHP workers, pooling, recycling, and process shutdown.
- The lab must use dynamically allocated loopback ports, bounded waits, process cleanup traps, and per-stage logs.
- Private signing keys and GitHub Actions secrets must never enter the lab.
- Offline mode must never silently download an artifact; missing cache entries are an explicit negative assertion.
- The lab must remain disposable and repeatable without modifying either source repository.

## Review Focus

- A release asset from the wrong tag or a draft/prerelease must fail before runtime startup; pinned by Task 3 release assertions.
- A signed catalog with a missing or mismatched digest must fail without publishing an executable; pinned by Task 3 catalog assertions.
- A fixture with no `bootstrap/app.php` or with a wrong bootstrap return value must fail before RoadRunner accepts traffic; pinned by Task 4 failure tests.
- A process that ignores graceful stop or leaves RoadRunner/worker descendants alive must fail cleanup; pinned by Task 5 lifecycle assertions.
- Invalid control/component configuration must fail startup without exposing configuration or secrets through metadata; pinned by Task 5 control-plane assertions.

---

### Task 1: Scaffold the external laboratory and result contract

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/README.md` — operator instructions, prerequisites, modes, and troubleshooting.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/.gitignore` — generated app state, downloads, logs, and results.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/config/defaults.env` — default Engine path, release tag, Framework path, and timeouts.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/common.sh` — WSL/Linux path, process, port, timeout, cleanup, and result helpers.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/common.ps1` — Windows equivalents with the same result fields and exit semantics.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/run-all.sh` — Bash orchestration entry point.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/run-all.ps1` — PowerShell orchestration entry point.

**Interfaces:**
- Produces `results/run.json` with mode, platform, engine version/commit, stage names, status, exit code, log path, and diagnostic fields.
- Produces `results/logs/<stage>.log` for every executed stage.
- Exposes `run_stage(name, command...)` in Bash and `Invoke-LabStage -Name -Script` in PowerShell.

- [ ] **Step 1: Initialize the external lab repository**

  Create the directory, initialize Git, and add only the scaffold files. Do not copy generated application or toolchain files.

- [ ] **Step 2: Implement shared stage/result helpers**

  Use strict Bash mode and PowerShell terminating errors. Allocate ports through loopback listeners, bound every wait, register cleanup traps, and write JSON only from structured stage results.

- [ ] **Step 3: Add a scaffold self-test**

  Run each entry point in `--list` mode and assert that it reports the same stage names without requiring PHP, Composer, RoadRunner, or GitHub credentials.

- [ ] **Step 4: Commit the scaffold**

  ```bash
  git add README.md .gitignore config scripts
  git commit -m "test: scaffold the Tusk Engine lab"
  ```

### Task 2: Generate the shared modern Framework fixture

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/fixture.sh` — WSL fixture generation and validation.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/fixture.ps1` — Windows fixture generation and validation.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/app/.gitkeep` — empty fixture marker; generated files remain ignored.
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/run-all.*` — register the fixture stage.

**Interfaces:**
- Consumes `TUSK_FRAMEWORK_PATH` and the selected Engine mode.
- Produces a clean `app/` containing `bootstrap/app.php`, `config/`, `routes/`, `public/index.php`, `composer.json`, and `tusk.json`.
- Produces `fixture.json` assertions proving no root `worker.php` and no generated `.tusk/runtime/worker.php` exist before Engine startup.

- [ ] **Step 1: Add fixture assertions**

  Assert the exact modern skeleton files, that `bootstrap/app.php` returns the Framework application contract, that routes use the documented router callback, and that a legacy root worker is absent.

- [ ] **Step 2: Generate from the coordinated Framework checkout**

  Use the Framework generator through its Composer autoload, configure a path repository for the local Framework checkout, and install dependencies into the lab only. Do not run Composer against the Engine repository.

- [ ] **Step 3: Configure deterministic endpoints**

  Add a root response, a health response, a request-correlation response, and a deliberate application error route. Keep responses independent of databases and external services.

- [ ] **Step 4: Run fixture-only verification**

  Run the fixture stage twice from a clean state. Expected: identical file contract and no Engine-generated worker before `tusk start`.

- [ ] **Step 5: Commit the fixture stage**

  ```bash
  git add app scripts/stages/fixture.* scripts/run-all.*
  git commit -m "test: generate a modern Tusk fixture"
  ```

### Task 3: Implement local-source and published-release acquisition

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/acquire.sh` — local/release binary and catalog acquisition for WSL/Linux.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/acquire.ps1` — local/release binary and catalog acquisition for Windows.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/catalog.sh` — digest, signature, release, and attestation assertions used by Bash.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/catalog.ps1` — PowerShell catalog assertions.
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/config/defaults.env` — pin the default release and artifact versions.

**Interfaces:**
- Produces `state/engine/tusk` or `state/engine/tusk.exe`, `state/catalog/toolchain-catalog.json`, and `state/catalog/toolchain-catalog.provenance.json`.
- Produces `engine.json` containing source type, tag/commit, release URL, asset digest, and verification status.
- In `published-release`, consumes `gh`, the release workflow identity, and the public trust anchor only.

- [ ] **Step 1: Add release rejection tests**

  Exercise wrong tag, draft/prerelease metadata, missing catalog, and missing provenance using local test inputs. Assert failure occurs before the runtime stage.

- [ ] **Step 2: Implement local-source acquisition**

  Build `./cmd/tusk` from the selected Engine checkout with isolated Go cache/temp directories and record the commit SHA. Never write build output into the Engine checkout.

- [ ] **Step 3: Implement published-release acquisition**

  Download the pinned release binary and catalog assets, verify release metadata, verify the catalog with the public trust anchor, verify catalog/provenance attestations with the expected workflow/tag, and verify selected artifact digests.

- [ ] **Step 4: Run acquisition tests**

  Run local-source mode without GitHub credentials and published mode with `gh auth status`; assert that each mode produces the same executable interface and result fields.

- [ ] **Step 5: Commit acquisition**

  ```bash
  git add config scripts/lib scripts/stages/acquire.*
  git commit -m "test: support local and published Engine acquisition"
  ```

### Task 4: Validate CLI, signed toolchain, and Composer setup

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/toolchain.sh`.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/toolchain.ps1`.
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/run-all.*` — register CLI and toolchain stages.

**Interfaces:**
- Consumes the acquired Engine, signed catalog, and generated fixture.
- Produces a fully provisioned `.tusk/toolchain.json` plus project-local PHP, Composer, and RoadRunner paths.

- [ ] **Step 1: Add CLI contract assertions**

  Assert `help`, `init`, `doctor`, `toolchain list`, and `toolchain pin` exit successfully with the documented output. Assert `setup --toolchain --offline` refuses a missing cache without downloading.

- [ ] **Step 2: Add clean online provisioning assertions**

  From a clean project state, pin PHP 8.3.35, Composer 2.10.3, and RoadRunner 2025.1.15, run `tusk setup --toolchain`, assert all three manifest paths are project-local, and validate PHP, Composer-through-PHP, and RoadRunner versions.

- [ ] **Step 3: Add cache reuse assertions**

  Remove installed tool paths while preserving the digest cache, rerun offline setup, and assert provisioning succeeds without network access.

- [ ] **Step 4: Run the stage in both modes**

  Expected: local-source and published-release produce the same toolchain manifest and executable checks. If a platform lacks a catalog artifact, report a precise matrix failure rather than silently falling back to system tools.

- [ ] **Step 5: Commit toolchain coverage**

  ```bash
  git add scripts/stages/toolchain.* scripts/run-all.*
  git commit -m "test: validate Engine toolchain provisioning"
  ```

### Task 5: Exercise RoadRunner, HTTP, control API, components, and lifecycle

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/runtime.sh`.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/runtime.ps1`.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/process-tree.sh`.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/process-tree.ps1`.

**Interfaces:**
- Consumes the fixture and provisioned toolchain.
- Produces runtime logs, generated `.tusk/runtime/worker.php`, HTTP assertions, control-plane assertions, and cleanup status.

- [ ] **Step 1: Add bounded startup assertions**

  Allocate distinct loopback ports for HTTP, control, status, RPC, and metrics; start `tusk start`; wait for readiness; assert exactly one Engine-owned RoadRunner process and one generated worker.

- [ ] **Step 2: Add HTTP and worker assertions**

  Assert the deterministic root and health responses, request correlation behavior, application error behavior, and that the generated worker references the Framework RoadRunner path without native NDJSON fallback.

- [ ] **Step 3: Add control-plane assertions**

  Assert versioned health, readiness, metadata, and Prometheus responses. Assert metadata includes component descriptors but excludes configuration values, secrets, and handler maps.

- [ ] **Step 4: Add component and lifecycle assertions**

  Assert first-party invocation/resilience configuration activates before traffic, reload does not lose the application contract, SIGTERM drains/stops cleanly, and no RoadRunner or worker descendant remains.

- [ ] **Step 5: Run runtime coverage twice**

  Run once with local-source and once with published-release. Expected: both modes pass HTTP/control/lifecycle assertions and leave only retained result logs.

- [ ] **Step 6: Commit runtime coverage**

  ```bash
  git add scripts/lib/process-tree.* scripts/stages/runtime.*
  git commit -m "test: exercise RoadRunner runtime lifecycle"
  ```

### Task 6: Add negative tests and repeatable cleanup

**Files:**
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/failure-modes.sh`.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/stages/failure-modes.ps1`.
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/lib/common.*` — preserve diagnostics and cleanup state.

**Interfaces:**
- Produces one result per negative scenario with expected failure marker, exit code, and proof that no runtime process remains.

- [ ] **Step 1: Add bootstrap failure scenarios**

  Temporarily remove `bootstrap/app.php` and replace it with a wrong return type. Assert non-zero startup, actionable path/type diagnostics, no HTTP acceptance, and no generated worker left active.

- [ ] **Step 2: Add platform/configuration failure scenarios**

  Test absent trusted catalog, unpinned tool, unavailable RoadRunner, port collision, malformed `tusk.json`, and invalid component field. Assert each fails before serving traffic.

- [ ] **Step 3: Add cleanup verification**

  Force a stage failure and assert the trap removes only lab-owned processes and temporary state. Preserve logs and the failing fixture for diagnosis.

- [ ] **Step 4: Run the negative suite twice**

  Expected: all failures are intentional, messages are stable enough to diagnose, and a following positive runtime run succeeds without manual repair.

- [ ] **Step 5: Commit failure coverage**

  ```bash
  git add scripts/lib/common.* scripts/stages/failure-modes.*
  git commit -m "test: cover Engine failure modes and cleanup"
  ```

### Task 7: Complete orchestration, documentation, and final audit

**Files:**
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/scripts/run-all.*` — stage ordering, mode selection, summary, and exit code.
- Modify: `C:/Users/wende/Projects/tusk/tusk-engine-lab/README.md` — exact Windows/WSL commands and troubleshooting.
- Create: `C:/Users/wende/Projects/tusk/tusk-engine-lab/results/.gitkeep`.

**Interfaces:**
- `run-all.sh --mode local-source|published-release` and `run-all.ps1 -Mode local-source|published-release` execute the same ordered stages.
- `--keep-state` preserves the fixture and downloaded artifacts for diagnosis; default cleanup removes generated state after successful runs.
- Exit code is non-zero if any required stage fails; skipped platform matrix entries are explicit in `run.json`.

- [ ] **Step 1: Wire the complete stage graph**

  Execute acquisition, fixture, CLI, toolchain, runtime, control/lifecycle, and failure stages in dependency order; stop before runtime if trust or toolchain validation fails.

- [ ] **Step 2: Add mode and platform documentation**

  Document prerequisites, WSL path conversion, GitHub CLI authentication for published mode, release/tag overrides, cleanup, retained logs, and the distinction between local-source and published-release.

- [ ] **Step 3: Run the full audit**

  Run both modes twice from clean state. Expected: a passing `results/run.json`, no leaked Engine/RoadRunner processes, and reproducible stage summaries.

- [ ] **Step 4: Self-review the lab**

  Check for hardcoded secrets, repository writes outside the lab, unbounded waits, fixed ports, hidden network fallback, and stale references to the legacy native runtime.

- [ ] **Step 5: Commit the final lab**

  ```bash
  git add .
  git commit -m "test: complete the Tusk Engine integration lab"
  ```

- [ ] **Step 6: Record findings**

  Open focused Engine or Framework issues only for reproducible failures discovered by the lab, including the exact mode, platform, commit/release, stage result, and retained log path.
