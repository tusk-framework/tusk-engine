# Engine components

The Engine activates its component registry before starting the control plane
or RoadRunner. A malformed component configuration, provider factory failure,
configuration failure, or startup health failure prevents serving and does not
publish a partial registry.

## Configuration

Add component settings to `tusk.json` under `components`. Component names and
fields are schema-validated; unknown component names, unknown fields, wrong
types, and out-of-range policy values fail startup.

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

The default resilience policy is bounded to a five-minute maximum deadline,
five total attempts, and the existing circuit-breaker bounds. Retries apply
only to inherently idempotent requests or requests carrying a non-empty
idempotency key; unknown and non-idempotent requests are attempted once.

## First-party providers

The default registry includes:

- `in-process-service-invocation`, a transport-free handler provider. Code can
  register a handler for a service name and use the stable
  `ServiceInvocationProvider` contract. It does not add HTTP, gRPC, or another
  network transport.
- `default-resilience`, which validates policy configuration and creates the
  existing deadline/retry/circuit `Invoker` for a substituted invocation
  provider.

Integrations can supply a replacement registration with the same capability
contract through Engine construction. Registration remains factory-based, so
providers are configured and health-checked as one atomic activation.

## Control metadata

When enabled, `GET /v1/metadata` exposes component descriptors only: names,
versions, schema declarations, capabilities, health behavior, and declared
secret field names. Configuration values, secret values, handler maps, policy
instances, and provider internals are never serialized.

Actors, cluster scheduling, placement, distributed coordination, and network
transport providers are intentionally deferred to later work.
