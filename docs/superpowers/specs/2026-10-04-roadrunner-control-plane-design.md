# RoadRunner Control Plane Integration

## Status

Design approved in conversation on 2026-10-04. Implementation is complete on
the RoadRunner control-plane branch and is ready for review.

## Context

Tusk Engine is intended to be the Go control plane for Tusk applications.
RoadRunner is the application runtime: it owns HTTP serving, PHP workers,
Goridge IPC, pooling, recycling, and request limits. The Engine must own
project configuration, runtime lifecycle, readiness, diagnostics, and
platform integration without recreating RoadRunner internals.

The current main branch already contains:

- deterministic RoadRunner v3 configuration projection;
- a generic runtime lifecycle manager with explicit states;
- toolchain diagnosis for PHP, Composer, and RoadRunner;
- a local versioned control API;
- a legacy native HTTP/NDJSON server and PHP worker pool that are still wired
  into tusk start.

The remaining architectural gap is that the documented RoadRunner control
plane is not yet the actual default execution path.

## Decision

tusk start will launch and supervise RoadRunner as a child process. The
Engine will not start the legacy native HTTP server or its PHP worker pool in
the normal start path.

The runtime data flow becomes:

    tusk CLI
       |
       v
    Tusk Engine control plane
       |-- validates config and resolves RoadRunner
       |-- renders an isolated RoadRunner config
       |-- starts and supervises the RoadRunner process
       |-- polls RoadRunner readiness
       |-- exposes Engine health/readiness/metadata/metrics
       |-- handles reload and graceful shutdown
       |
       v
    RoadRunner
       |-- HTTP
       |-- PHP workers
       |-- Goridge IPC
       |-- pooling and recycling
       v
    Tusk PHP application

RoadRunner is the only supported runtime path for tusk start after this
integration. The native implementation remains in the repository temporarily
as migration-era code, but is no longer selected, started, or advertised as
a parallel platform architecture.

## Goals

- Make the runtime manager the real owner of the RoadRunner child process.
- Make startup fail clearly when RoadRunner or its generated configuration is
  invalid.
- Expose readiness only after RoadRunner reports an HTTP worker ready.
- Preserve the existing control API contract and map its responses to runtime
  state rather than native worker state.
- Make graceful stop, forced stop, process exit, and reload observable and
  testable.
- Keep all RoadRunner control sockets loopback-only by default.
- Make process and probe dependencies injectable so unit tests do not require
  PHP or RoadRunner binaries.

## Non-goals

- Reimplementing RoadRunner's worker pool, IPC, request limits, or recycling.
- Building a reverse proxy or gateway in front of RoadRunner.
- Adding service discovery, distributed aggregation, or a Dapr-like component
  model in this slice.
- Supporting a silent fallback from RoadRunner to the native runtime.
- Adding remote runtime mutation endpoints to the control API.

## Components

### Runtime process adapter

Add a production ProcessFactory implementation around os/exec and a
RoadRunner-specific process adapter.

The adapter will:

- start the resolved RoadRunner executable with serve and the generated
  configuration path;
- preserve the project root as the child working directory;
- stream child stdout/stderr through the Engine logger;
- expose Wait so unexpected process exit transitions the manager to failed;
- request graceful shutdown using the platform-supported RoadRunner process
  signal and fall back to Kill when the grace deadline expires;
- implement reload through the RoadRunner reset mechanism using the same
  project-local configuration and loopback control settings.

The generic runtime manager remains independent of exec.Cmd. The adapter
owns operating-system details and is covered by fake-process tests.

### RoadRunner configuration projection

Extend the existing deterministic projection with the Engine-owned status
and reload/control settings required for lifecycle management:

- HTTP address and worker configuration remain derived from Config;
- the status plugin listens on a loopback address;
- the status plugin is available for health and readiness probes;
- any RoadRunner RPC/reset address is loopback-only;
- user-supplied project values are validated before rendering;
- generated configuration is written to a temporary or project-local runtime
  file owned by the Engine and removed after shutdown when safe.

The projection must never serialize the control bearer token or arbitrary
Engine configuration into RoadRunner output.

### Readiness probe

Introduce an injectable probe that calls RoadRunner's status plugin:

- liveness probe: /health?plugin=http;
- readiness probe: /ready?plugin=http;
- loopback-only target derived from the generated runtime config;
- bounded request timeout and startup deadline;
- retry with a short backoff until ready, process exit, or deadline;
- classify failures as starting, not_ready, process_failed, or timeout
  without exposing arbitrary child output through the public control response.

RoadRunner's status plugin returns HTTP 200 when a worker can serve traffic and
503 when no worker is ready, which gives the Engine a runtime-native readiness
signal rather than a synthetic PHP-pool count.

### Runtime snapshot provider

The runtime manager will implement the control API's read-only snapshot
provider, or expose a dedicated adapter owned by the runtime package.

The snapshot maps:

- created and starting to Engine starting;
- successful readiness probe to Engine running/ready;
- graceful shutdown to stopping;
- completed shutdown to stopped;
- child exit, failed startup, or probe deadline to Engine failed/unavailable.

The control contract will add an explicit failed Engine state and stable
process_failed and timeout readiness reasons. Failed and stopped runtimes are
not healthy; starting, running, and stopping runtimes remain live for
diagnostic purposes.

The configured worker count is reported as desired capacity. Readiness is
based on RoadRunner's status response. Exact active/ready worker counts and
worker crash counters are not fabricated when RoadRunner does not expose them
through the selected probe. The snapshot therefore gains a worker-counts-known
flag; the RoadRunner adapter reports desired capacity and readiness while
marking exact active/total counts as unknown. Expanded diagnostics remain a
follow-up observability issue.

### CLI lifecycle

runServerWithConfig will become a runtime orchestration flow:

1. validate the full project and control configuration;
2. resolve the RoadRunner executable through the toolchain resolver;
3. render and persist the RoadRunner configuration;
4. construct the runtime manager and control API with a shared snapshot
   provider;
5. start the control listener;
6. start RoadRunner;
7. poll readiness and mark the runtime ready;
8. wait for SIGINT/SIGTERM or a runtime/serve failure;
9. stop RoadRunner gracefully under one shutdown context;
10. stop the control listener and clean generated runtime files.

The control API must be available while the runtime is starting so operators
can distinguish a live-but-not-ready Engine from a failed process.

Unexpected RoadRunner exit must be propagated to the CLI and must not leave
the control API reporting ready.

## Configuration contract

The existing canonical Tusk configuration remains the source of application
settings. The integration may add only explicit runtime lifecycle settings
needed to avoid port collisions and configure bounded probe behavior. New
settings must have loopback-safe defaults and deterministic validation.

The runtime settings are grouped under a RuntimeConfig value and include:

- RoadRunner status address, defaulting to 127.0.0.1:2114;
- RoadRunner RPC/reset address, defaulting to tcp://127.0.0.1:6001;
- startup timeout, with a bounded default;
- readiness probe interval, with a bounded default.

The Engine writes generated configuration under .tusk/runtime using a unique
0600 file and removes only files carrying its generated-runtime marker. A
user-authored .rr.yaml is never overwritten or deleted.

RoadRunner executable selection follows this order:

1. project-local path declared in .tusk/toolchain.json;
2. system rr discovered by the toolchain resolver;
3. fail with a diagnostic that identifies the missing tool and suggests
   tusk doctor.

The Engine must not download, replace, or silently select an untrusted
RoadRunner binary during tusk start.

## Error and shutdown semantics

- Invalid config fails before a process or listener is started.
- RoadRunner start failure closes the control listener and preserves the
  original start error.
- Readiness timeout stops the child, marks the runtime failed, and returns a
  bounded diagnostic.
- A child process that exits unexpectedly transitions to failed and causes the
  CLI to terminate with a non-zero error.
- Graceful stop is idempotent.
- If graceful stop exceeds its deadline, the adapter kills the child and
  reports whether the forced stop succeeded.
- Generated config cleanup never removes a user-authored file.

## Testing strategy

### Unit tests

- deterministic RoadRunner projection includes status/reload settings;
- runtime configuration validates loopback status/RPC addresses and bounded
  startup/probe timing;
- invalid addresses, ports, worker counts, and missing binaries fail safely;
- process adapter builds the expected command and working directory;
- runtime manager transitions through start, ready, failed, reload, and stop;
- readiness probe handles 200, 503, transport failure, malformed response,
  process exit, and deadline expiration;
- runtime snapshot mapping never reports ready before the probe succeeds;
- failed and stopped runtime snapshots are not healthy;
- worker-counts-known prevents fabricated RoadRunner worker counters;
- control API responses remain versioned and secret-free.

### Integration tests

- CLI orchestration starts control before RoadRunner;
- a fake RoadRunner process becomes ready only after the probe is released;
- failed startup closes the control listener;
- unexpected process exit makes readiness unavailable;
- shutdown stops both RoadRunner and control API;
- generated configuration is cleaned up after normal and failed startup.

### Verification

    go test ./...
    go test -race ./...
    go vet ./...
    go build ./cmd/tusk
    git diff --check

Tests must use fake process factories and HTTP probe transports where possible.
An end-to-end RoadRunner test may be added to CI only when a pinned,
trustworthy RoadRunner binary is available in the workflow.

## Rollout

The change is intentionally a hard architectural transition for tusk start:
missing or invalid RoadRunner is an actionable startup error, not a reason to
fall back to the native server. Documentation, diagnostics, and examples must
all describe RoadRunner as the runtime.

The native packages remain isolated until a separate cleanup issue removes
them after downstream migration confidence is established.

## Follow-ups

- expose richer worker lifecycle counters using RoadRunner-native diagnostics;
- add secured operational commands for reload/status when CLI subcommands are
  introduced;
- evaluate RoadRunner RPC-backed component integrations after the runtime
  boundary is stable;
- remove the legacy native implementation once migration criteria are met.

## References

- RoadRunner health and readiness status plugin:
  https://docs.roadrunner.dev/docs/logging-and-observability/health
- RoadRunner lifecycle commands and graceful shutdown:
  https://docs.roadrunner.dev/docs/app-server/cli
- RoadRunner RPC and Goridge control boundary:
  https://docs.roadrunner.dev/docs/php-worker/rpc
