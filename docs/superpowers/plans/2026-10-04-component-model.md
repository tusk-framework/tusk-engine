# Versioned Component Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a validated, versioned component registry with service invocation and resilience contracts while preserving provider substitution and control-plane safety.

**Architecture:** Add a dependency-free `internal/components` package split into descriptor/schema/registry and invocation/resilience units. Registry activation is the pre-serving gate; providers are supplied by factories and resolved by capability. Extend only safe control metadata, leaving actors, cluster scheduling, and network transports for later issues.

**Tech Stack:** Go 1.23, standard library, table-driven tests, `context`, `time`, `sync`.

**Spec:** `docs/superpowers/specs/2026-10-04-component-model-design.md`

## Global Constraints

- Component schema version is `v1`.
- Retry `MaxAttempts` counts total provider calls and is bounded to five.
- Retry only inherently idempotent or idempotency-keyed invocations.
- Invalid component configuration fails activation before the registry can resolve providers.
- No actors, cluster scheduling, remote plugin loading, or network transport implementation.
- Control metadata includes descriptors only; it never includes configuration or secret values.

## Review Focus

- A component factory/configure/health failure must not leave any provider resolvable; covered by Task 1 activation atomicity tests.
- A malformed secret declaration or redaction alias must not leak its value; covered by Task 1 schema tests.
- A non-idempotent invocation must not be retried even when policy allows retries; covered by Task 2 tests.
- Concurrent half-open circuit calls must allow only one probe; covered by Task 2 circuit tests.
- Metadata serialization must not expose provider configuration or secret values; covered by Task 3 control tests.

---

### Task 1: Versioned descriptors, schema validation, and registry lifecycle

**Files:**
- Create: `internal/components/model.go`
- Create: `internal/components/schema.go`
- Create: `internal/components/registry.go`
- Test: `internal/components/model_test.go`
- Test: `internal/components/schema_test.go`
- Test: `internal/components/registry_test.go`

**Interfaces:**
- Produces `components.Descriptor`, `Capability`, `Schema`, `Configuration`, `Provider`, `Registration`, `ProviderFactory`, and `Registry`.
- `Registry.Activate(context.Context, map[string]Configuration) error`, `Registry.Resolve(name string, capability Capability) (Provider, error)`, `Registry.Health(context.Context, name string) error`, and `Registry.Descriptors() []Descriptor` are the only lifecycle surface later tasks consume.

- [ ] **Step 1: Write failing descriptor and schema tests.** Cover valid `v1` descriptors; missing name/version/schema/capability; duplicate capabilities; unsupported field types; required fields; unknown keys; primitive type mismatches; secret declaration mismatch; and redaction that preserves non-secret values.
- [ ] **Step 2: Run focused tests to verify failure.** Run `go test ./internal/components`; expect compile failures for the new package/API.
- [ ] **Step 3: Implement model and schema validation.** Keep errors contextual and deterministic; accept all Go integer types and JSON `float64` values only when integral for integer fields.
- [ ] **Step 4: Write failing registry lifecycle tests.** Prove duplicate registration rejection, configuration validation before factory use, startup health gating, on-demand/disabled health, capability mismatch, resolve-before-activate rejection, provider replacement through factories, and atomic failed activation.
- [ ] **Step 5: Implement registry lifecycle.** Validate declarations in `NewRegistry`; instantiate/configure/health all providers in temporary state; publish the state only after every component succeeds.
- [ ] **Step 6: Run Task 1 tests and commit.** Run `go test -count=1 ./internal/components`; expected all component tests pass. Commit `feat: add versioned component registry contracts`.

### Task 2: Service invocation and resilience executor

**Files:**
- Create: `internal/components/invocation.go`
- Create: `internal/components/resilience.go`
- Test: `internal/components/invocation_test.go`
- Test: `internal/components/resilience_test.go`

**Interfaces:**
- Consumes `Provider` and `CapabilityServiceInvocation` from Task 1.
- Produces `ServiceInvocationProvider`, `InvocationRequest`, `InvocationResponse`, `IdempotencyMode`, `InvocationPolicy`, `Invoker`, `CircuitBreaker`, `CircuitState`, `ErrCircuitOpen`, and `ErrInvocationDeadline`.

- [ ] **Step 1: Write failing policy and circuit tests.** Cover bounded policy validation, closed→open after threshold, open fast-fail, one deterministic half-open probe, successful close, failed re-open, and concurrent probe exclusion.
- [ ] **Step 2: Run circuit tests to verify failure.** Run `go test ./internal/components -run 'Test(InvocationPolicy|Circuit)'`; expect missing API/behavior failures.
- [ ] **Step 3: Implement policy and circuit breaker.** Use a mutex, injected `now` seam for package tests, one half-open permit, and bounded thresholds/reset duration.
- [ ] **Step 4: Write failing invocation tests.** Cover deadline propagation, successful call, transient failures with exponential backoff and total-attempt bound, no retry for unspecified/non-idempotent requests, retry with idempotency key, no retry after context deadline, and circuit-open provider suppression.
- [ ] **Step 5: Implement the invoker.** Apply the derived deadline, execute one logical call through the breaker, retry only when request safety allows, cap backoff, and return stable wrapped errors without logging payloads.
- [ ] **Step 6: Run Task 2 tests and commit.** Run `go test -count=1 ./internal/components`; expected all tests pass. Commit `feat: add resilient service invocation contract`.

### Task 3: Control-plane descriptors and operator guidance

**Files:**
- Modify: `internal/control/metadata.go`
- Test: `internal/control/metadata_test.go`
- Modify: `internal/control/server_test.go`
- Modify: `README.md`
- Create: `docs/components.md`

**Interfaces:**
- Consumes `components.Descriptor` and `Registry.Descriptors()` from Task 1.
- Keeps `/v1/metadata` version `v1`; adds a safe `components` array containing descriptor metadata and secret field names only.

- [ ] **Step 1: Write failing metadata tests.** Assert component descriptors serialize with name/version/schema/capabilities/health/secret field names and that a configuration value never appears.
- [ ] **Step 2: Implement safe metadata projection.** Copy and sort descriptors; expose no mutable maps or configuration values.
- [ ] **Step 3: Add focused control and guidance documentation.** Document activation-before-serving, provider substitution, schema/secret rules, deadline/retry/circuit defaults, and idempotency guidance; explicitly list actors and cluster scheduling as deferred.
- [ ] **Step 4: Run focused tests and commit.** Run `go test -count=1 ./internal/control ./internal/components`; expected pass. Commit `docs: expose component model safely through control plane`.

### Task 4: Full verification and branch handoff

**Files:**
- No planned product files; fix only issues found by verification.

- [ ] **Step 1: Run the complete verification set.** Run `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/tusk`, and `git diff --check`; all must pass.
- [ ] **Step 2: Review scope and security.** Confirm no actor/scheduler implementation, no configuration/secret leakage, bounded retries/deadline, atomic activation, and no main/push/PR mutation.
- [ ] **Step 3: Perform a fresh whole-branch review.** Review the diff against the fork point; fix Critical/Important findings with RED→GREEN tests and record deferred minors.
- [ ] **Step 4: Commit any verification fixes locally.** Keep all commits on `codex/component-model` and report their SHAs.
