# Extension-Aware PHP Toolchain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Configure every project-local PHP process consistently and report the actual PHP extension capabilities of the selected runtime.

**Architecture:** Consume the authenticated PHP profile metadata produced by the portable-bundles plan. Keep application requirements in Composer metadata; this plan reports actual loaded modules and configures only project-local PHP INI paths, while Composer-based requirement comparison is implemented by the managed-Composer plan.

**Tech Stack:** Go 1.23 standard library, `os/exec`, existing toolchain report/environment, signed PHP metadata.

**Spec:** `docs/superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md`

## Global Constraints

- Prefer project-local, pinned, verified toolchains; never mutate the global `PATH` or replace system-managed tools.
- Keep acquisition explicit. `tusk start`, `tusk dev`, ordinary diagnostics, and package commands do not perform an implicit network installation.
- Keep one source of truth for dependency requirements and lockfiles; Composer metadata owns application requirements.
- Fail with actionable diagnostics when a platform, PHP extension, or native library combination is unsupported.
- The Engine sets PHP configuration variables only in child-process environments and never edits machine-wide `php.ini`.
- Unsupported extension/platform combinations fail closed; do not download arbitrary PECL DLLs or shared objects.

## Review Focus

- PHP emits startup warnings before the module list: report the warning and do not parse contaminated output as a valid capability list; test invalid INI/module startup.
- Module names differ only by case or alias: normalize case and compare canonical extension names; test mixed-case inputs and duplicate required names.
- A project path escapes the project root: reject relative configuration paths outside the project; test `..`, symlink, and absolute-path policies.
- A system PHP has a user/global INI loaded: do not override system-profile behavior; only set project-local INI variables for a managed runtime; test both profiles.
- Catalog metadata claims an extension not actually loaded: report the mismatch and fail readiness; test missing and unexpected module sets.

---

### Task 1: Persist and resolve the managed PHP configuration directory

**Files:**
- Modify: `internal/toolchain/setup.go` — publish project-local PHP configuration beside the installed runtime only after artifact verification.
- Modify: `internal/toolchain/environment.go` — add process-local `PHPRC` and `PHP_INI_SCAN_DIR` for a project-local PHP runtime, preserving inherited values for system/Docker profiles.
- Modify: `internal/toolchain/environment_test.go` — assert environment isolation and OS-specific path handling.
- Test: `internal/toolchain/setup_test.go`, `internal/toolchain/environment_test.go`.

**Interfaces:**
- Consumes: `Tool.Path`, `Tool.Source`, and verified `Artifact.PHP` metadata from the portable-bundles plan.
- Produces: existing `BuildEnvironment(root, report, base)` includes deterministic local PHP configuration variables for all child commands; parent environment remains unchanged.

- [ ] **Step 1: Write failing tests** for project-local PHP INI, scan directory, Windows path case/separator, system-profile preservation, Docker profile preservation, and no mutation of the supplied base environment.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run 'TestBuildEnvironment.*PHP' -count=1`; verify the new assertions fail.
- [ ] **Step 3: Implement** a project-local configuration layout under `.tusk/toolchain/php/<version>/<target>/`; use only safe paths under the installed runtime and never infer config paths from untrusted catalog strings.
- [ ] **Step 4: Run** `go test ./internal/toolchain -run 'Test(BuildEnvironment|Setup).*' -count=1`; expect the new and existing environment/setup tests to pass.
- [ ] **Step 5: Commit** `feat(toolchain): isolate project PHP configuration`.

### Task 2: Inspect and compare PHP's actual loaded extension set

**Files:**
- Create: `internal/toolchain/php_extensions.go` — module inspection, canonicalization, and comparison helpers.
- Create: `internal/toolchain/php_extensions_test.go` — parser and comparison tests.
- Modify: `internal/toolchain/doctor.go` — attach a PHP capability report without downloading tools or changing configuration.
- Modify: `internal/cli/cli.go` — render extension-profile, loaded-module, and actionable mismatch diagnostics.
- Modify: `internal/cli/toolchain_test.go` — human and JSON output tests.

**Interfaces:**
- Produces: `ExtensionReport{Profile string; Declared []string; Loaded []string; Missing []string; Unexpected []string; Warnings []string}` and `InspectPHPModules(ctx context.Context, phpPath string, env []string, declared []string) (ExtensionReport, error)`.
- The inspector invokes the resolved PHP with a fixed `-r` probe that emits JSON from `get_loaded_extensions()`; it never evaluates project PHP or Composer plugins.

- [ ] **Step 1: Write failing tests** for JSON module output, case normalization, duplicate names, missing modules, unexpected modules, malformed JSON, timeout/process failure, and startup warnings.
- [ ] **Step 2: Run** `go test ./internal/toolchain ./internal/cli -run 'Test(PHP.*Extension|Doctor.*Extension)' -count=1`; verify new cases fail.
- [ ] **Step 3: Implement** the inspector with context cancellation and the project-local environment from Task 1. Treat non-empty PHP startup warnings as diagnostics; do not silently mark a broken runtime ready.
- [ ] **Step 4: Integrate `doctor`** with human/JSON-safe extension data and a remediation that names the missing module and supported profile. Keep `doctor` read-only and network-free.
- [ ] **Step 5: Run** `go test ./internal/toolchain ./internal/cli -count=1`; expect all existing and new tests to pass.
- [ ] **Step 6: Commit** `feat(toolchain): diagnose PHP extension capabilities`.

### Task 3: Guard project-local INI and artifact metadata consistency

**Files:**
- Modify: `internal/toolchain/setup.go` — validate extracted config files and write managed provenance/metadata atomically.
- Modify: `internal/toolchain/installer.go` — reject malformed/unexpected bundle paths while retaining archive traversal protection.
- Modify: `internal/toolchain/setup_test.go`, `internal/toolchain/installer_test.go` — tampering and path tests.

**Interfaces:**
- Consumes: Task 1's PHP configuration layout and catalog metadata.
- Produces: a managed installation is usable only when its executable, profile metadata, config, and verified installation record agree.

- [ ] **Step 1: Write failing tests** for missing INI, escaped scan path, post-install metadata mismatch, and atomic failure that leaves the previous manifest/runtime untouched.
- [ ] **Step 2: Run** `go test ./internal/toolchain -run 'Test(Setup|Installer).*PHP' -count=1`; verify failure.
- [ ] **Step 3: Implement** validation before updating `.tusk/toolchain.json`; never overwrite a user-supplied PHP path or configuration.
- [ ] **Step 4: Run** `go test ./internal/toolchain -count=1` and `git diff --check`; expect all checks to pass.
- [ ] **Step 5: Commit** `fix(toolchain): validate managed PHP bundle layout`.

## Execution Order

Depends on the portable PHP bundles/catalog metadata plan. The Composer plan consumes `ExtensionReport` for platform-check diagnostics but remains the authority for required extensions.
