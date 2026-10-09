# Managed Composer and PHP Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure Composer, framework commands, bootstrap validation, scripts, and RoadRunner workers all use the PHP selected by the project toolchain and validate real platform requirements.

**Architecture:** Reuse the existing Composer PHAR resolution through the selected PHP process and remove remaining direct uses of `cfg.PhpBinary`/ambient `PATH` from managed project execution. Keep Composer's solver and lockfile semantics untouched. Add a thin `tusk check-platform-reqs` wrapper and a pre-start runtime check using Composer's real platform check.

**Tech Stack:** Go 1.23 standard library, existing toolchain `ResolveCommand`/`BuildEnvironment`, Composer PHAR, RoadRunner.

**Spec:** `docs/superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md`

## Global Constraints

- Composer remains the dependency solver and `composer.lock` authority.
- The Engine's selected project PHP is used for Composer PHAR and RoadRunner workers.
- Keep acquisition explicit; commands do not download missing tools implicitly.
- Keep one source of truth for PHP extension requirements: application and Framework `composer.json` platform constraints.
- Do not use Composer `config.platform` as evidence that real extensions are installed.
- Preserve system/Docker profiles and process-local environment isolation.

## Review Focus

- Composer configured as PHAR versus executable wrapper: use the resolved PHP only for PHARs; test both artifact forms.
- System Composer or PHP conflicts with pinned project binaries: assert project toolchain wins and global `PATH` is unchanged.
- Missing lockfile/vendor tree: provide a clear partial-check/skipped state instead of a false pass; test all installation states.
- A runtime-only missing extension is masked by `config.platform`: use the real PHP process and test a simulated platform config.
- A failed pre-start check still starts RoadRunner: assert startup is not invoked and the diagnostic retains Composer's failure context.

---

### Task 1: Route every Engine-owned PHP command through the resolved toolchain

**Files:**
- Modify: `internal/cli/cli.go` — resolve PHP for bootstrap validation and generic framework commands; provide the isolated environment to scripts and child processes.
- Modify: `internal/cli/toolchain_exec.go` — centralize PHP/Composer process construction without duplicating resolution.
- Modify: `internal/roadrunner/config.go` — project the absolute resolved PHP executable into the generated RoadRunner worker command.
- Modify: `internal/cli/bootstrap_test.go`, `internal/cli/toolchain_test.go`, `internal/roadrunner/config_test.go`.

**Interfaces:**
- Consumes: `toolchain.ResolveExecutable`, `toolchain.ResolveCommand`, and `toolchain.BuildEnvironment`.
- Produces: a CLI helper `buildResolvedPHPCommand(cfg *config.Config, report toolchain.Report, args []string) (*exec.Cmd, error)` used for PHP CLI/bootstrap/framework commands; Composer continues through `buildToolchainCommand` and `ResolveCommand`.

- [ ] **Step 1: Write failing tests** proving bootstrap validation, generic framework commands, and RoadRunner's worker command use a project-local absolute PHP path even when `php` on PATH is a different version.
- [ ] **Step 2: Run** `go test ./internal/cli ./internal/roadrunner -run 'Test.*(ManagedPHP|PHPBinary|Bootstrap)' -count=1`; verify the failures reproduce the current direct-config/PATH behavior.
- [ ] **Step 3: Implement** the helper and inject its resolved path/environment into all PHP child processes. Scripts invoked through `tusk run` inherit the same project-local tool directories without changing the Engine process.
- [ ] **Step 4: Run** `go test ./internal/cli ./internal/roadrunner -count=1`; expect all existing and new tests to pass.
- [ ] **Step 5: Commit** `fix(cli): use the managed PHP runtime consistently`.

### Task 2: Add the explicit real-platform validation command

**Files:**
- Modify: `internal/cli/cli.go` — route `tusk check-platform-reqs [--no-dev]` to Composer's built-in command.
- Create: `internal/cli/platform_check.go` — argument parsing, invocation, and structured result handling.
- Create: `internal/cli/platform_check_test.go` — command and failure tests.

**Interfaces:**
- Produces: `runPlatformCheck(cfg *config.Config, args []string, output io.Writer, run commandRunner) error`; it executes `composer check-platform-reqs` through the same resolved PHP/Composer command builder.
- `commandRunner` is `type commandRunner func(*exec.Cmd) error`; production uses `cmd.Run`, tests inject a deterministic runner that captures executable, arguments, environment, stdout/stderr, and exit error.
- `--no-dev` is passed through unchanged to Composer; no custom dependency resolver or duplicate extension parser is introduced.

- [ ] **Step 1: Write failing tests** for no flags, `--no-dev`, unknown/repeated flags, missing Composer, missing PHP for PHAR, command exit code propagation, and system-profile fallback.
- [ ] **Step 2: Run** `go test ./internal/cli -run TestPlatformCheck -count=1`; verify the new cases fail.
- [ ] **Step 3: Implement** the narrow wrapper and preserve Composer's stdout/stderr and exit status.
- [ ] **Step 4: Run** `go test ./internal/cli -count=1`; expect all tests to pass.
- [ ] **Step 5: Commit** `feat(cli): add Composer platform requirement check`.

### Task 3: Block RoadRunner startup on an actual runtime platform failure

**Files:**
- Modify: `internal/cli/cli.go` — before publishing the worker/starting RoadRunner, run Composer's real platform check when a lockfile or installed vendor tree exists.
- Modify: `internal/cli/platform_check.go` — expose runtime-only (`--no-dev`) and full development checks.
- Modify: `internal/cli/bootstrap_test.go`, `internal/cli/cli_test.go`, `internal/cli/platform_check_test.go`.
- Modify: `docs/guides/project-runtime.md` — document `tusk check-platform-reqs` and startup behavior.

**Interfaces:**
- Consumes: Task 1's resolved process factory and Task 2's platform-check runner.
- Produces: `tusk start` checks `composer check-platform-reqs --no-dev` against the real selected PHP before RoadRunner launch; when no lockfile/vendor tree exists, it reports a non-successful partial/unknown check and explains the required setup command rather than claiming validation passed.

- [ ] **Step 1: Write failing tests** for a passing runtime check, missing production extension, dev-only missing extension (must not block production start), no installed dependencies, and Composer check process failure.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Test(Start|PlatformCheck).*' -count=1`; verify startup incorrectly proceeds in the missing-extension case.
- [ ] **Step 3: Implement** the preflight before worker/config publication and RoadRunner process creation. Keep full dev checks available through the explicit CLI command.
- [ ] **Step 4: Run** `go test ./internal/cli ./internal/roadrunner -count=1`; expect startup and package-command tests to pass.
- [ ] **Step 5: Commit** `feat(runtime): verify Composer platform before startup`.

### Task 4: Coordinate Framework platform requirements

**Files:**
- Separate repository PR: `tusk-framework/composer.json` — declare the Framework's mandatory runtime `ext-*` requirements based on actual API use.
- Separate repository PR: `tusk-framework/tusk-cli/src/Generator/ProjectGenerator.php` and a new toolchain stub — generate project-local PHP, Composer, and RoadRunner pins matching the supported catalog.
- Separate repository PR: `tusk-framework/tusk-cli/tests/Generator/ProjectGeneratorTest.php` — assert the generated toolchain manifest and platform requirements.
- Separate repository tests/CI: verify install and runtime check against the Framework's supported PHP matrix.
- Engine integration test: generated skeleton's lockfile reflects the Framework platform constraints.

**Interfaces:**
- Consumes: a reviewed Framework-side list of required PHP extensions.
- Produces: Composer platform metadata that Engine setup/check/start can trust, plus a generated `.tusk/toolchain.json` pinned to the supported PHP/Composer/RoadRunner versions. Optional integrations remain optional and must not become hard dependencies of the base Framework package.

- [ ] **Step 1: Audit Framework API calls and package dependencies** to distinguish mandatory runtime extensions from optional driver/integration extensions.
- [ ] **Step 2: Add failing Framework Composer/CI assertions** for missing mandatory `ext-*` constraints.
- [ ] **Step 3: Add only proven mandatory `ext-*` constraints** to Framework `composer.json`; do not duplicate an Engine-maintained list.
- [ ] **Step 4: Update the generator** to create `.tusk/toolchain.json` with versions present in the supported official catalog; assert the manifest in `ProjectGeneratorTest.php` and preserve `.gitignore` rules.
- [ ] **Step 5: Run** `composer validate --strict` and `vendor/bin/phpunit --filter ProjectGeneratorTest` in the Framework checkout, then run the existing full `vendor/bin/phpunit --testdox --colors=always` suite on the supported PHP matrix. Open a separate Framework PR before declaring Engine pre-start validation complete.

## Execution Order

Task 4 must merge before Task 3 is treated as complete for the default Framework skeleton. Tasks 1 and 2 can proceed once the portable PHP bundles and extension-aware environment plan are available. Task 3 depends on Tasks 1-2 and the Framework metadata PR.
