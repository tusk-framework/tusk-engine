# Versioned Component Registry and Capability Model

## Context

The Engine has a versioned control API and a RoadRunner lifecycle boundary,
but no common contract for replaceable platform providers. Framework packages
already contain discovery and resilience ideas; the Engine needs a small,
language-neutral boundary that can validate providers before traffic is served.

This slice implements the foundation requested by issue #7. Service
invocation and resilience are executable first-party contracts. Discovery,
secrets, configuration, state, pub/sub, bindings, and jobs are declared as
capability vocabulary for compatibility and future providers. Actors and
cluster scheduling are explicitly out of scope.

## Goals

- Define a versioned descriptor for a component: name, implementation version,
  schema version, capabilities, configuration schema, health behavior, and
  secret field names.
- Register provider factories by stable component name and reject duplicate or
  malformed declarations deterministically.
- Validate component configuration, instantiate providers, configure them, and
  run required startup health checks before the registry becomes ready.
- Keep provider implementations replaceable through small Go interfaces.
- Provide service invocation with a bounded deadline, bounded retry attempts,
  idempotency-aware retry guidance, and a concurrency-safe circuit breaker.
- Expose safe component descriptors through the existing versioned control
  metadata response without exposing configuration values or secret values.

## Non-goals

- No actors, cluster scheduling, placement, durable jobs, or pub/sub runtime.
- No network transport implementation; an HTTP/gRPC provider can implement the
  invocation contract later.
- No dynamic plugin loading or remote provider process protocol.
- No automatic retries for an operation whose idempotency is unknown.

## Contracts

### Descriptor and schema

The public package boundary is `internal/components`. `Descriptor` requires a
non-empty name, semantic implementation version, `v1` schema version, at least
one capability, a valid health behavior, and a schema whose secret fields are
also declared in `SecretFields`. Capability names are stable strings such as
`service.invocation`, `resilience`, `discovery`, `secrets`, `configuration`,
`state`, `pubsub`, `bindings`, and `jobs`.

`Schema` describes known fields with primitive types (`string`, `integer`,
`boolean`, `duration`, and `object`), requiredness, and secret status.
Validation rejects missing required fields, unknown fields, type mismatches,
and secret declaration mismatches. `Configuration.Redacted` returns a copy
with secret values replaced by `[REDACTED]`; it is the only configuration form
safe for diagnostics.

### Registry lifecycle

`Registration` pairs a descriptor with a `ProviderFactory`. `Registry` owns
the provider instances and has a closed/ready lifecycle:

1. `NewRegistry` validates declarations and uniqueness.
2. `Activate(ctx, configurations)` validates all configurations, creates and
   configures every provider, and runs health checks for components declaring
   startup health.
3. Only after activation does `Resolve` return a provider. Failed activation
   leaves the registry closed and does not expose a partially configured set.

On-demand health components are checked through `Registry.Health`; disabled
health behavior is explicit. A resolved provider must advertise the requested
capability, allowing callers to depend on a stable capability contract rather
than a concrete provider.

### Service invocation and resilience

`ServiceInvocationProvider` extends `Provider` with `Invoke(context.Context,
InvocationRequest) (InvocationResponse, error)`. Requests carry service,
method, path, headers, body, idempotency mode, and optional idempotency key.

`Invoker` wraps a service invocation provider with `InvocationPolicy`:

- `Deadline` is required and bounded; the derived context never outlives it.
- `Retry.MaxAttempts` counts total provider calls and is capped at five.
- Exponential backoff is capped by `Retry.MaxBackoff` and stops at context
  cancellation/deadline.
- Unspecified/non-idempotent operations are attempted once. Retries require
  either an inherently idempotent operation or a provider-enforced idempotency
  key (`IdempotentWithKey` with a non-empty key).
- A circuit breaker counts one failed invocation after its retry loop, opens at
  a bounded failure threshold, and permits one half-open probe after the reset
  timeout. A successful probe closes it.

Provider errors are returned unchanged through a wrapped stable sentinel where
possible (`ErrCircuitOpen`, `ErrInvocationDeadline`). No response body or
configuration value is logged by this foundation.

### Control plane

`control.Metadata` gains safe component descriptors. `/v1/metadata` remains
versioned as `v1`; descriptors include names, versions, schema version,
capabilities, health behavior, and secret field names only. Configuration
values, secret values, and provider internals are never serialized.

## Data flow

```text
tusk.json / caller config
        |
        v
Registry.New -> descriptor/schema validation
        |
Registry.Activate -> config validation -> provider configure -> startup health
        |
        +---- ready registry --------------------+
        |                                         |
        v                                         v
Resolve(capability)                         control metadata (safe descriptors)
        |
        v
Invoker: deadline -> circuit -> idempotency-aware bounded retries -> provider
```

## Error behavior

- Invalid descriptor, duplicate name, invalid schema, missing/unknown/wrongly
  typed configuration, factory error, configure error, or required health
  failure prevents activation and returns a stable contextual error.
- Resolving before activation, an unknown component, or an unsupported
  capability is rejected without invoking a provider.
- Circuit-open calls fail fast without invoking the provider.
- Context cancellation and deadline are never retried.

## Compatibility and testing

Every first-party contract in this slice has table-driven unit tests for valid
and invalid descriptors, schema validation/redaction, registry lifecycle and
health behavior, provider substitution/capability checks, retry bounds and
idempotency, deadline propagation, circuit transitions, and safe control
metadata serialization. The package has no external runtime dependency, so
fake providers and deterministic clocks can prove behavior without a network.

