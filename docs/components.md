# Engine components

Tusk Engine activates registered components before it starts the control
server or RoadRunner. Component configuration errors, provider construction
errors, configuration errors, and startup health failures therefore fail
`tusk start` before the Engine can serve traffic. Activation is atomic: a
failed activation does not publish a partially configured registry.

## Configuration

Component settings live under `components` in `tusk.json`. Component names
and fields are validated against the registered `v1` schema. Unknown
component names or fields fail startup; configuration values are never
included in `/v1/metadata`.

```json
{
  "components": {
    "default-resilience": {
      "deadline": "5s",
      "max_attempts": 3,
      "initial_backoff": "50ms",
      "max_backoff": "500ms",
      "failure_threshold": 3,
      "reset_timeout": "30s"
    }
  }
}
```

Duration fields use Go duration syntax. Retry attempts count total provider
calls and are bounded to five. Retries run only for inherently idempotent
requests or requests carrying a provider-enforced idempotency key. Unknown
and non-idempotent requests run once. The deadline bounds the whole logical
invocation, including retries and backoff. The circuit breaker counts one
logical failed invocation, fails fast while open, and permits one half-open
probe after its reset timeout.

## First-party providers

The default registry includes:

- `in-process-service-invocation`, which implements the service-invocation
  contract with registered Go handlers. It intentionally does not choose an
  HTTP, gRPC, or other network transport. A missing handler returns a stable
  unavailable error.
- `default-resilience`, which validates the bounded invocation policy and
  constructs the existing deadline/retry/circuit-breaker invoker for any
  compatible service-invocation provider.

Provider factories are explicit registry registrations. Applications and
tests can substitute a provider while retaining the same capability contract;
application code does not need to depend on a concrete provider type.

## Control metadata

When enabled, `/v1/metadata` exposes component descriptors only: name,
implementation version, schema version, capabilities, health behavior, and
declared secret field names. It does not expose component configuration,
secret values, handler maps, resilience policy values, or provider internals.

Actors, cluster scheduling, placement, distributed coordination, dynamic
plugins, and network transports are deferred to later component-model work.
