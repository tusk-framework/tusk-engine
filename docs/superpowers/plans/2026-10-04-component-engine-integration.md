# Engine Component-Model Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make component activation a pre-serving Engine lifecycle gate, add replaceable first-party invocation/resilience providers, and expose safe configuration/metadata.

**Architecture:** Add `internal/engine.Engine` as the composition boundary around the existing runtime manager and control server. Extend `config.Config` with component sections, add transport-free first-party providers in `internal/components`, and make CLI startup delegate lifecycle ownership to Engine while retaining executable resolution and signal handling.

**Tech Stack:** Go 1.23, standard library, `context`, `sync`, `time`, `net/http`, table-driven tests.

**Spec:** `docs/superpowers/specs/2026-10-04-component-engine-integration-design.md`

## Global Constraints

- Component activation completes before the control server or RoadRunner starts.
- Component schema version remains `v1`; unknown component sections fail activation.
- Retry attempts remain bounded to five total provider calls and only safe/idempotent requests retry.
- Deadlines bound the whole logical invocation; circuit breakers remain concurrency-safe and fail fast when open.
- Metadata exposes descriptors only; component configuration, provider state, handler maps, and secret values never serialize.
- No actors, cluster scheduling, placement, distributed coordination, dynamic plugins, or speculative transports.
- Work remains on the isolated `codex/issue7-component-engine` branch; do not push, open a PR, merge, or modify `main`.

## Review Focus

- Invalid component configuration must fail before either control bind or RoadRunner process creation; covered by Engine startup-order tests.
- Unknown component sections must not be silently ignored; covered by config/registry activation tests.
- First-party in-process invocation must not leak handler/provider internals and must return a stable error for missing services; covered by provider tests.
- Resilience configuration must preserve bounded policy validation and idempotency-aware retries; covered by provider/invocation tests.
- Failed startup after control bind must close the control listener and leave no running RoadRunner process; covered by Engine cleanup tests.

---

### Task 1: Configuration and registry activation hardening

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/components/registry.go`
- Modify: `internal/components/registry_test.go`

**Interfaces:**
- `config.Config.Components map[string]components.Configuration` serialized as `components`.
- `Registry.Activate` rejects any configuration section without a registered component before creating providers.

- [ ] **Step 1: Write failing configuration tests.** Assert `tusk.json` loads nested component maps and that nested values survive default/overlay merging without sharing mutable maps.
- [ ] **Step 2: Run focused config tests and verify RED.** Run `go test -count=1 ./internal/config -run 'Test(LoadConfig|DefaultConfig).*Component'`; expected failure because `Config.Components` is absent.
- [ ] **Step 3: Implement component config loading/merging.** Add the field, initialize an empty map, and deep-copy overlay component maps in `mergeConfig`.
- [ ] **Step 4: Write failing unknown-section activation test.** Assert an unknown component config fails before any registered factory is called and the registry remains not ready.
- [ ] **Step 5: Implement registry unknown-section validation.** Validate configuration section names in sorted order before provider creation while preserving atomic activation on all other failures.
- [ ] **Step 6: Run Task 1 tests and commit.** Run `go test -count=1 ./internal/config ./internal/components`; commit `feat: validate engine component configuration`.

### Task 2: First-party invocation and resilience providers

**Files:**
- Create: `internal/components/firstparty.go`
- Create: `internal/components/firstparty_test.go`
- Modify: `internal/components/invocation.go`
- Modify: `internal/components/invocation_test.go`

**Interfaces:**
- `FirstPartyRegistrations() []Registration` returns `in-process-service-invocation` and `default-resilience` registrations.
- `InProcessServiceInvocationProvider` supports `Register(service string, handler InvocationHandler) error` and the `ServiceInvocationProvider` contract.
- `ResilienceProvider` exposes `Policy() InvocationPolicy` and `NewInvoker(ServiceInvocationProvider) (*Invoker, error)`.

- [ ] **Step 1: Write failing provider tests.** Cover descriptor/schema validation, deterministic defaults, typed duration/integer configuration, handler registration/invocation, missing-handler sentinel error, and policy-based Invoker construction.
- [ ] **Step 2: Run provider tests and verify RED.** Run `go test -count=1 ./internal/components -run 'Test(FirstParty|InProcess|DefaultResilience)'`; expected missing provider APIs/types.
- [ ] **Step 3: Implement the in-process provider.** Use a mutex-protected handler map and defensive request/response copies; return `ErrServiceUnavailable` without exposing handler details when no handler exists.
- [ ] **Step 4: Implement the resilience provider.** Parse supported configuration values, apply bounded defaults, validate through `InvocationPolicy`, and construct `Invoker` instances without adding transport behavior.
- [ ] **Step 5: Add failing regression tests for substitution and retry safety.** Register a custom provider under the same capability and assert the invoker still honors non-idempotent/no-key no-retry behavior.
- [ ] **Step 6: Run Task 2 tests and commit.** Run `go test -count=1 ./internal/components`; commit `feat: add first-party component providers`.

### Task 3: Engine lifecycle integration

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`
- Modify: `internal/cli/cli.go`

**Interfaces:**
- `engine.Options` supplies an optional registry, process factory/spec, readiness probe, metrics handler, and startup/shutdown timing seams.
- `engine.New(*config.Config, Options) (*Engine, error)` constructs the lifecycle boundary.
- `(*Engine).Start(context.Context) error`, `(*Engine).Stop(context.Context) error`, `(*Engine).Wait(context.Context) error`, `(*Engine).Snapshot() control.RuntimeSnapshot`, and `(*Engine).Registry() *components.Registry` are the lifecycle surface.

- [ ] **Step 1: Write failing Engine lifecycle tests.** Use fake process/readiness/control dependencies to prove invalid component config prevents factory/control startup, successful activation precedes runtime readiness, injected providers are used, and a readiness failure stops already-started resources.
- [ ] **Step 2: Run Engine tests and verify RED.** Run `go test -count=1 ./internal/engine`; expected package/API failures.
- [ ] **Step 3: Implement Engine construction and activation gate.** Build the default registry when no registry is supplied, activate it first, and construct control metadata only from registry descriptors.
- [ ] **Step 4: Implement startup/cleanup ordering.** Start control and await bind, start runtime and await readiness, track control errors, and make `Stop` safe for partial/failed startup.
- [ ] **Step 5: Refactor CLI startup to Engine.** Keep RoadRunner executable/config-file projection and signal handling in CLI; delegate activation, control lifecycle, runtime start, readiness, and cleanup to Engine.
- [ ] **Step 6: Run focused lifecycle/control tests and commit.** Run `go test -count=1 ./internal/engine ./internal/cli ./internal/control`; commit `feat: gate engine startup on component activation`.

### Task 4: Metadata, documentation, and integration tests

**Files:**
- Modify: `internal/control/metadata.go`
- Modify: `internal/control/metadata_test.go`
- Modify: `README.md`
- Create: `docs/components.md`
- Modify: `tusk.json`

**Interfaces:**
- `/v1/metadata` remains version `v1` and receives descriptors from the active Engine registry only.
- Documentation defines the `components` configuration shape, first-party provider names, activation-before-serving behavior, substitution guidance, and deferred capabilities.

- [ ] **Step 1: Write failing metadata/integration tests.** Assert descriptors are present after Engine activation, configuration values/policy values/handler details are absent, and the JSON shape remains versioned.
- [ ] **Step 2: Implement any required safe metadata projection.** Preserve defensive copies and sort component descriptors/capabilities for deterministic output; never add configuration fields.
- [ ] **Step 3: Document operator configuration and scope.** Add a focused component guide, README links/examples, and a safe sample configuration without secrets.
- [ ] **Step 4: Run focused tests and commit.** Run `go test -count=1 ./internal/engine ./internal/control ./internal/config ./internal/components`; commit `docs: document engine component integration`.

### Task 5: Full verification and local handoff

**Files:**
- No planned product files; only test-backed fixes found during verification.

- [ ] **Step 1: Run the requested complete verification.** Run `go test -count=1 ./...`, `go vet ./...`, and `git diff --check`; read all exit codes/output.
- [ ] **Step 2: Run race/build checks.** Run `go test -race ./...` and `go build ./cmd/tusk`.
- [ ] **Step 3: Review the full diff against `main`.** Verify lifecycle order, atomic activation, substitution, bounded resilience semantics, metadata safety, documentation scope, and no forbidden actor/scheduler/transport work.
- [ ] **Step 4: Apply only test-backed verification fixes.** Each fix gets RED→GREEN plus a full suite run; commit locally if needed.
- [ ] **Step 5: Record final SHA/files/tests/limitations.** Confirm branch/worktree state, local commits, and that no push/PR/merge/main mutation occurred.
