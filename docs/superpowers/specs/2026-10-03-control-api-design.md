# Tusk Engine Local Control API

## Context

The Tusk Engine already owns the HTTP traffic server and PHP worker pool, but it has no stable operational API. Operators and local tooling cannot distinguish an engine that has not started, an engine whose workers crashed, and an engine that is ready to serve traffic. The framework now exposes provider-neutral runtime diagnostics, so the Engine needs a local control-plane boundary that can report process and worker state without exposing application payloads or secrets.

This specification implements the first part of tusk-engine issue #6. It deliberately keeps the API local and versioned; remote administration, fleet aggregation, and authentication delegation remain later control-plane work.

## Goals

- Expose versioned health, readiness, metadata, and metrics endpoints.
- Make readiness reflect the managed Engine and PHP worker pool.
- Keep the default control listener local-only and safe on Windows and Unix.
- Require explicit configuration and a bearer token for non-local exposure.
- Provide immutable, race-safe pool state snapshots.
- Make startup, worker crash, restart, and shutdown states deterministic and testable.
- Keep the traffic server and control API as separate HTTP surfaces with separate configuration.

## Non-goals

- Replacing RoadRunner supervision or implementing Goridge.
- Remote control commands, configuration mutation, or process execution over HTTP.
- Querying arbitrary PHP application endpoints from the control API.
- Returning raw configuration, environment variables, request payloads, headers, cookies, or secrets.
- Distributed health aggregation or service discovery.

## Architecture

Tusk Engine
├── Traffic server: configured public address/port
├── Worker pool: process lifecycle and request dispatch
└── Control API: local loopback address/port
    ├── /v1/healthz
    ├── /v1/readyz
    ├── /v1/metadata
    └── /v1/metrics

The control API is implemented in internal/control. It receives a read-only RuntimeSnapshot provider instead of reaching into worker.Pool internals. The pool owns state transitions and publishes snapshots under a mutex. The CLI starts and stops both servers using one root context; stopping the traffic server must not leave the control listener running.

## Configuration

The control API is disabled unless configured. This prevents a new management endpoint from appearing unexpectedly in existing deployments.

Configuration example:

{
  "control": {
    "enabled": true,
    "address": "127.0.0.1",
    "port": 9091,
    "token": ""
  }
}

Rules:

- enabled defaults to false.
- The default address is 127.0.0.1; loopback addresses do not require a token.
- An address that is not loopback requires a non-empty token. Startup fails before binding if it is missing.
- The token is accepted only as Authorization: Bearer <token> and compared with constant-time equality.
- The token is never returned by metadata or error responses.
- Unix sockets are deferred because the Engine must provide the same default behavior on Windows and Unix.

## Response contracts

All responses are JSON, include Content-Type: application/json, and use stable versioned fields. Error responses use:

{
  "version": "v1",
  "error": "not_ready",
  "message": "no worker is ready"
}

### /v1/healthz

Reports whether the Engine process is alive. It returns HTTP 200 while the process has a valid runtime snapshot, including during startup and shutdown. It returns HTTP 503 only when the control state is unavailable, not when the application is merely not ready.

{
  "version": "v1",
  "status": "ok",
  "engine": "running",
  "timestamp": "2026-10-03T12:00:00Z"
}

### /v1/readyz

Reports whether the Engine can accept application traffic. It returns HTTP 200 only when:

- the Engine lifecycle is running;
- the pool has at least one ready worker;
- the pool has not recorded a fatal startup error.

It returns HTTP 503 for starting, stopping, stopped, crashed, and zero-ready-worker states.

{
  "version": "v1",
  "status": "ready",
  "engine": "running",
  "workers": {
    "desired": 4,
    "ready": 4,
    "active": 0
  },
  "timestamp": "2026-10-03T12:00:00Z"
}

### /v1/metadata

Returns safe identity and capability information only:

{
  "version": "v1",
  "engine": {
    "name": "tusk-engine",
    "version": "0.1.0",
    "go_version": "go1.23.0",
    "os": "windows",
    "arch": "amd64"
  },
  "runtime": {
    "worker_count": 4,
    "timeout_seconds": 30
  },
  "capabilities": ["http", "persistent-workers", "metrics"],
  "control": {
    "remote_access": false
  }
}

Paths, environment values, arbitrary tusk.json fields, tokens, and dependency maps are not returned.

### /v1/metrics

The existing Prometheus endpoint is moved behind the control server and remains a scrape-only endpoint. It is disabled with the control API by default and is protected by the same token when the control listener is non-local. Existing metric names remain stable; adding worker lifecycle gauges and counters is a separate implementation slice covered by issue #3.

## Runtime snapshot

RuntimeSnapshot is an immutable value object containing:

- engine lifecycle: starting, running, stopping, or stopped;
- readiness reason: starting, no_workers, worker_crashed, ready, or stopping;
- desired, ready, active, and total worker counts;
- worker restart and crash counters;
- startup timestamp and last state-change timestamp;
- optional sanitized last error category, never an arbitrary error string.

The pool updates the snapshot at worker spawn, worker checkout/check-in, worker exit, restart failure, successful pool start, and shutdown. HTTP handlers copy one snapshot and never hold the pool mutex while encoding a response.

## Lifecycle and failure behavior

- Before Pool.Start, health is 200 and readiness is 503 with starting.
- If initial worker creation fails, startup returns the original error and readiness is 503 with no_workers or worker_crashed.
- If one worker crashes after a healthy start, the pool increments the crash counter and readiness remains 200 while another worker is ready; it becomes 503 only when no worker is ready.
- A successful restart restores ready capacity without resetting historical crash/restart counters.
- During graceful shutdown, health remains 200, readiness becomes 503, and both listeners close under the same shutdown context.
- Control handler failures cannot terminate the traffic server or worker supervisor.

## Testing strategy

- Unit-test configuration defaults, loopback detection, remote-token validation, and constant-time bearer authentication.
- Unit-test immutable pool snapshots and every lifecycle transition without sleeping.
- Use fake worker processes or injected process factories for child-not-started, child-crashed, restart, and ready states.
- Use httptest to verify status codes, stable JSON fields, redaction, and unauthorized remote requests.
- Add an integration test that starts the control server with a fake snapshot provider and shuts it down through context cancellation.
- Run go test ./..., go vet ./..., and the existing CI workflow.

## Rollout

The feature is opt-in and local-only by default. Documentation will describe /v1/healthz and /v1/readyz as local orchestration endpoints and explicitly warn that a non-loopback bind is an administrative surface requiring a token. Issue #6 remains open until implementation and CI are complete; issue #3 can consume the control server without changing its security defaults.
