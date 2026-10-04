# Engine Component-Model Integration

## Context

The merged component foundation defines versioned descriptors, schema
validation, atomic registry activation, service-invocation semantics, and
safe control metadata. The Engine startup path does not yet own that
lifecycle: `tusk start` builds the RoadRunner manager and control server
directly, and `tusk.json` cannot configure components.

This slice advances the foundation into the Engine control plane while
preserving the existing RoadRunner boundary. Component activation is a
pre-serving gate. The Engine supplies transport-free first-party providers
for in-process service invocation and configurable resilience policy; external
HTTP/gRPC transports remain future work.

## Goals

- Add a testable Engine lifecycle that activates the component registry before
  the control server or RoadRunner can serve traffic.
- Load component configuration from `tusk.json`, validate it against the
  registered component schemas, and reject unknown component sections.
- Register replaceable first-party service-invocation and resilience
  providers without adding network transports.
- Make the configured resilience provider construct the existing bounded,
  idempotency-aware invoker with deadlines, retries, and circuit breaking.
- Expose only active component descriptors through `/v1/metadata`; never
  serialize component configuration or provider state.
- Preserve provider substitution through explicit registry/Engine injection
  for tests and future integrations.
- Document configuration, lifecycle guarantees, provider contracts, and
  deferred actors/cluster scheduling.

## Non-goals

- No actors, cluster scheduling, placement, or distributed coordination.
- No HTTP, gRPC, message-bus, plugin-process, or other speculative transport.
- No dynamic plugin loading or configuration-driven code execution.
- No component configuration values in control metadata, logs, or errors.
- No changes to `main`, no push, no pull request, and no merge.

## Design

### Engine lifecycle

Introduce `internal/engine.Engine` as the composition boundary around the
existing runtime manager and control server. Construction accepts the
validated application configuration plus runtime/process dependencies and
allows an explicitly supplied registry; the production constructor uses the
first-party registry.

`Engine.Start(ctx)` performs these steps in order:

1. Validate the control/runtime configuration.
2. Activate all registered components using `Config.Components`.
3. Build metadata from the registry descriptors.
4. Start and await the control server, when enabled.
5. Start RoadRunner and wait for its existing readiness probe.

Any activation or startup failure stops resources already started and leaves
the Engine unavailable. `Engine.Stop(ctx)` is idempotent and stops RoadRunner
and control resources. The CLI becomes a thin adapter for executable
resolution, signal handling, and the existing process wait loop.

### Configuration

`config.Config.Components` is a map from stable component name to
`components.Configuration`, serialized as:

```json
{
  "components": {
    "default-resilience": {
      "deadline": "5s",
      "max_attempts": 3,
      "initial_backoff": "50ms",
      "max_backoff": "500ms",
      "circuit_failure_threshold": 5,
      "circuit_reset_timeout": "30s"
    }
  }
}
```

The registry rejects unknown component names and delegates field validation
to each descriptor schema before constructing any provider. Provider
configuration is not copied into metadata.

### First-party providers

`internal/components/firstparty.go` provides two registrations:

- `in-process-service-invocation`: a handler-based provider implementing the
  existing `ServiceInvocationProvider`. It is deliberately transport-free;
  callers register handlers in code, and an absent handler returns a stable
  unavailable error.
- `default-resilience`: a provider implementing a small
  `ResilienceProvider` interface that exposes a validated `InvocationPolicy`
  and constructs `Invoker` instances for a service provider. Its configuration
  uses bounded duration/integer fields and has deterministic safe defaults.

Both descriptors use schema version `v1`, startup health, and explicit
capabilities. A caller can replace either registration with another provider
factory while retaining the same capability contract.

### Control metadata

The Engine passes `Registry.Descriptors()` to `control.Metadata` only after
activation succeeds. The existing defensive metadata projection remains the
serialization boundary: names, versions, schema declarations, capabilities,
health behavior, and secret field names may be exposed; configuration values,
handler maps, policy values, and provider internals may not.

## Error behavior

- Invalid component configuration fails `Engine.Start` before control or
  RoadRunner starts.
- Factory, configure, or startup-health errors fail activation atomically;
  no partially activated provider is resolvable.
- A control bind or RoadRunner startup/readiness failure cleans up resources
  already started and returns a contextual error.
- Invocation semantics remain those of the foundation: retry count is bounded,
  only safe/idempotent requests retry, deadlines bound the whole logical call,
  and circuit-open calls fail fast.

## Testing

TDD tests cover configuration loading and unknown sections, first-party
provider schemas/behavior, Engine activation ordering and cleanup, injected
provider substitution, metadata redaction, and the existing component
invocation/resilience invariants. Focused tests run before the complete
`go test ./...` and `go vet ./...` verification requested by the issue.
