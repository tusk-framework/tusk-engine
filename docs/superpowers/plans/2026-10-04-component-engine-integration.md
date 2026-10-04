# Component Engine Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the versioned component registry a tested Engine startup gate with safe configuration/metadata and transport-neutral first-party provider implementations.

**Architecture:** Add strict component configuration to `config.Config`, first-party provider helpers in `internal/components`, and an injected lifecycle coordinator in `internal/engine`. The CLI composes the default registry, passes its configuration into the coordinator before control/RoadRunner readiness, and projects only registry descriptors into control metadata.

**Tech Stack:** Go 1.23, standard library, existing `context`, `time`, `sync`, `errors.Join`, and table-driven tests.

**Spec:** `docs/superpowers/specs/2026-10-04-component-engine-integration-design.md`

## Global Constraints

- Component activation must complete before the control plane or RoadRunner starts serving.
- A configuration, factory, configure, or startup-health failure must not publish a partial registry or start a service.
- No actors, cluster scheduling, placement, distributed coordination, or network transport provider.
- Service-invocation retries remain bounded and only apply to idempotent or non-empty-keyed operations.
- Invocation deadlines and circuit-breaker behavior continue to use the existing `Invoker` and `CircuitBreaker` contracts.
- Component configuration values and secret values never appear in control metadata or error logging.
- The built-in resilience provider defaults are 5s deadline, 3 total attempts, 50ms initial backoff, 500ms maximum backoff, threshold 3, and reset timeout 30s.
- All work stays on `codex/issue7-component-engine`; no push, PR, merge, or main mutation.

## Review Focus

- A component activation failure after a provider factory has run must leave both the registry unresolved and the runtime/control seams unstarted; Task 2 tests the full startup gate.
- A malformed resilience duration or numeric configuration must fail before provider construction; Task 1 tests strict provider configuration.
- A function-backed provider with a nil handler must be rejected without a panic; Task 1 tests constructor validation.
- A control-plane startup failure after component activation must stop no unstarted runtime and must not leak a goroutine; Task 2 tests cleanup/error propagation.
- Metadata must remain descriptor-only even when the configured component map contains nested secret values; Task 3 tests serialized output and deep-copy behavior.

---

### Task 1: Configuration and transport-neutral first-party providers

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/components/firstparty.go`
- Create: `internal/components/firstparty_test.go`

**Interfaces:**
- Consumes: existing `components.Provider`, `Registration`, `ServiceInvocationProvider`, `InvocationPolicy`, and `Registry.Activate` schema validation.
- Produces: `config.Config.Components map[string]components.Configuration`; `components.InvocationHandler`; `components.FuncServiceInvocationProvider`; `components.NewFuncServiceInvocationProvider`; `components.ResilienceProvider`; `components.ResilienceRegistration`; and `components.DefaultRegistrations`.

- [ ] **Step 1: Write failing configuration tests.** Add a `tusk.json` fixture containing `components.tusk.resilience` overrides and assert `loadConfigFromDir` preserves string durations and integer settings; add a test that a malformed non-object component entry fails JSON loading.
- [ ] **Step 2: Run the focused configuration tests to verify RED.** Run `go test ./internal/config -run 'Test(LoadConfig.*Components|DefaultConfig.*Components)'`; expect compilation failure because `Config.Components` is absent.
- [ ] **Step 3: Add `Config.Components` and overlay handling.** Use `map[string]components.Configuration` with `json:"components,omitempty"`; copy the overlay map into the final config without changing existing composer/script precedence or defaults.
- [ ] **Step 4: Run the focused configuration tests to verify GREEN.** Run the same command and confirm all new and existing config tests pass.
- [ ] **Step 5: Write failing first-party provider tests.** Cover nil handler rejection, function provider invocation/descriptor identity, default registration containing `tusk.resilience`, default resilience policy, valid overrides, invalid duration/type/range errors, and `NewInvoker` substitution through the resilience provider.
- [ ] **Step 6: Run the focused provider tests to verify RED.** Run `go test ./internal/components -run 'Test(FirstParty|Resilience|FuncService)'`; expect missing types/functions or behavior failures.
- [ ] **Step 7: Implement `firstparty.go`.** Define the transport-neutral function adapter and the resilience provider. Parse only the declared schema fields, apply the exact bounded defaults, validate the resulting `InvocationPolicy`, and expose `Policy()` plus `NewInvoker(ServiceInvocationProvider)` without adding a network client.
- [ ] **Step 8: Run the package suite and commit Task 1.** Run `go test -count=1 ./internal/components ./internal/config`; expected all tests pass. Commit `feat: add Engine component configuration and providers`.

### Task 2: Atomic Engine lifecycle coordinator

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`

**Interfaces:**
- Consumes: `components.Registry`, `components.Configuration`, and `runtime.ReadinessProbe`.
- Produces: `engine.Runtime` (`Start`, `WaitReady`, `Stop`), `engine.ControlPlane` (`Start`, `WaitReady`, `Stop`), `engine.Options`, `engine.New`, `(*Engine).Start(context.Context) (<-chan error, error)`, `(*Engine).Stop(context.Context) error`, `(*Engine).Registry() *components.Registry`, and `(*Engine).ControlErrors() <-chan error`.

- [ ] **Step 1: Write failing lifecycle tests.** Use real component registries with test providers and recording runtime/control seams to assert activation precedes control/runtime start, successful start waits for runtime readiness, activation failure starts neither seam, control failure cleans up without starting runtime, runtime start/readiness failure cleans up, and repeated `Stop` is safe.
- [ ] **Step 2: Run the focused lifecycle tests to verify RED.** Run `go test ./internal/engine`; expect package/API compilation failures.
- [ ] **Step 3: Implement option validation and coordinator construction.** Require a registry, runtime, readiness probe, positive startup/probe intervals, and a control plane when control is enabled; default the control readiness timeout to 5 seconds.
- [ ] **Step 4: Implement `Engine.Start`.** Activate the registry first, asynchronously start/await the enabled control plane, start the runtime, and await its probe. On every post-activation error, stop only services that started, close the control error path, and return a contextual error.
- [ ] **Step 5: Implement `Engine.Stop` and accessors.** Stop runtime and control in cleanup order, attempt both applicable stops, return `errors.Join` for multiple failures, make repeated calls harmless, and expose the registry/control-error channel without mutable configuration.
- [ ] **Step 6: Run lifecycle tests and the package suite.** Run `go test -count=1 ./internal/engine`; expected all lifecycle tests pass with no goroutine or race symptoms. Commit `feat: gate Engine startup on component activation`.

### Task 3: CLI/control-plane integration and operator documentation

**Files:**
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `internal/control/metadata.go`
- Modify: `internal/control/metadata_test.go`
- Modify: `README.md`
- Create: `docs/components.md`

**Interfaces:**
- Consumes: `components.DefaultRegistrations`, `components.Registry.Descriptors`, `engine.New`, and `config.Config.Components` from Tasks 1–2.
- Produces: CLI startup integration in `runServerWithConfig`; descriptor-only metadata from the active default registry; documented `tusk.json` component configuration and provider substitution workflow.

- [ ] **Step 1: Write failing integration/metadata tests.** Add a CLI seam test proving component activation errors are returned before attempting RoadRunner resolution; add metadata tests that nested component configuration and secrets are absent, descriptors are copied, and descriptor fields remain present.
- [ ] **Step 2: Run focused tests to verify RED.** Run `go test ./internal/cli ./internal/control -run 'Test(Component|Metadata)'`; expect missing registry/lifecycle wiring or failing assertions.
- [ ] **Step 3: Integrate the coordinator into `runServerWithConfig`.** Construct the default registry before the control server, pass `cfg.Components`, use registry descriptors in `control.Metadata`, replace direct control/runtime startup and cleanup with `engine.Engine`, and preserve existing RoadRunner resolution, readiness, signal handling, and disabled-control behavior.
- [ ] **Step 4: Harden safe metadata projection.** Keep `/v1/metadata` at version `v1`, deep-copy descriptor slices/maps, sort component descriptors by name, and ensure no configuration map is reachable from the response structure.
- [ ] **Step 5: Write operator documentation.** Add `docs/components.md` and a concise README link/section covering activation ordering, the `components` JSON shape, default resilience bounds, idempotency/deadline/circuit behavior, function/provider substitution, safe metadata, and explicit actor/cluster/transport deferrals.
- [ ] **Step 6: Run focused integration tests and commit Task 3.** Run `go test -count=1 ./internal/cli ./internal/control ./internal/components ./internal/config ./internal/engine`; expected pass. Commit `feat: integrate components with Engine control lifecycle`.

### Task 4: Full verification and local handoff

**Files:**
- Modify only files needed to fix issues found by verification; no unrelated refactor.

- [ ] **Step 1: Run the complete verification set.** Run `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/tusk`, and `git diff --check`; every command must exit 0.
- [ ] **Step 2: Review the branch against the original `main` fork point.** Confirm atomic activation, startup ordering, safe metadata/configuration, substitution, bounded retries, idempotency, deadlines, circuit breakers, and absence of actors/scheduling/transports.
- [ ] **Step 3: Fix any verification findings with TDD.** For each finding, add a failing regression test, watch it fail, make the minimal fix, rerun focused tests, then rerun the full suite.
- [ ] **Step 4: Commit verification fixes locally and report final evidence.** Keep all commits local on `codex/issue7-component-engine`; report the final SHA, changed files, commands/results, and limitations. Do not push, open a PR, merge, or alter `main`.
