# Tusk Engine Local Control API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add an opt-in, versioned, local control API that reports Engine health, worker readiness, safe metadata, and protected Prometheus metrics.

**Architecture:** Keep the control API in internal/control with provider-neutral snapshot models. The worker pool publishes immutable snapshots under its existing mutex; the control server consumes a read-only provider and never reaches into pool internals. The CLI owns the lifecycle of the traffic server, control server, and pool under one shutdown context.

**Tech Stack:** Go 1.23, net/http, encoding/json, crypto/subtle, Prometheus client_golang, httptest, Go testing.

**Spec:** docs/superpowers/specs/2026-10-03-control-api-design.md

## Global Constraints

- The control API is disabled unless configured.
- The default control address is 127.0.0.1 and the default control port is 9091.
- A non-loopback control address requires a non-empty bearer token and constant-time comparison.
- Tokens, environment values, raw configuration, request payloads, headers, cookies, and secrets must never appear in control responses.
- Readiness is HTTP 200 only when the Engine is running, at least one worker is ready, and no fatal startup error is recorded.
- Health remains HTTP 200 during startup and graceful shutdown while a valid runtime snapshot exists.
- Existing Prometheus metric names remain stable.
- The control API must behave consistently on Windows and Unix; Unix sockets are out of scope.
- No remote mutation, process execution, service discovery, or distributed aggregation is added.

## Review Focus

- Control is disabled by default: existing traffic-server behavior must not gain an exposed management endpoint accidentally — Task 1 and Task 4 tests.
- A non-loopback address without a token: configuration must fail before any listener binds — Task 1 tests.
- A worker crashes while another worker remains ready: readiness stays healthy while crash/restart counters increase — Task 2 tests.
- A request carries an invalid, malformed, or wrong bearer token: the control API returns 401 without leaking token details — Task 3 tests.
- Metadata contains an executable path or arbitrary config field: the response must include only the explicitly approved safe fields — Task 3 tests.

---

### Task 1: Add validated control configuration and snapshot contracts

Files:
- Modify: internal/config/config.go
- Create: internal/control/model.go
- Create: internal/control/config.go
- Test: internal/config/config_test.go
- Test: internal/control/model_test.go
- Test: internal/control/config_test.go

Interfaces:
- config.Config gains Control ControlConfig.
- config.ControlConfig contains Enabled bool, Address string, Port int, and Token string.
- config.ControlConfig.Validate() error applies the loopback/token rules and rejects invalid ports.
- control.SnapshotProvider exposes Snapshot() control.RuntimeSnapshot.
- control.RuntimeSnapshot contains EngineState, ReadinessReason, DesiredWorkers, ReadyWorkers, ActiveWorkers, TotalWorkers, WorkerCrashes, WorkerRestarts, StartedAt, StateChangedAt, and LastErrorCategory.
- control.RuntimeSnapshot is copied by value and exposes Ready() bool and Healthy() bool helpers.

- [ ] Step 1: Write failing configuration tests.

Test exact behavior:
- DefaultConfig sets control disabled, address 127.0.0.1, and port 9091.
- A loopback enabled config with an empty token validates.
- A non-loopback enabled config without a token fails with a stable validation error.
- A non-loopback enabled config with a token validates.
- Ports below 1 or above 65535 fail.

- [ ] Step 2: Run focused tests to verify failure.

Run: go test ./internal/config ./internal/control

Expected: FAIL because ControlConfig, RuntimeSnapshot, and their validation do not exist.

- [ ] Step 3: Implement the configuration and model.

Keep control model types independent from worker.Pool and HTTP handlers. Use time.Time values internally; serialization decisions belong to the control handler. Treat empty or whitespace-only addresses as the loopback default.

- [ ] Step 4: Add failing model tests and make them pass.

Assert that a ready snapshot is healthy and ready, a starting snapshot is healthy but not ready, and a stopped or zero-worker snapshot is not ready. Assert that copies do not share mutable maps or slices.

Run: go test ./internal/config ./internal/control

Expected: PASS.

- [ ] Step 5: Commit.

Run:
    git add internal/config/config.go internal/config/config_test.go internal/control/model.go internal/control/config.go internal/control/model_test.go internal/control/config_test.go
    git commit -m "feat: define local control contracts"

### Task 2: Publish worker-pool lifecycle snapshots

Files:
- Modify: internal/worker/pool.go
- Modify: internal/worker/pool_test.go
- Create: internal/worker/snapshot_test.go

Interfaces:
- worker.Pool implements control.SnapshotProvider.
- Pool.Snapshot() returns a control.RuntimeSnapshot copy without holding the mutex while callers serialize it.
- Pool marks lifecycle transitions through private updateSnapshot helpers.
- Pool exposes no worker process handles or error strings through the public snapshot.

- [ ] Step 1: Write failing pool snapshot tests.

Use deterministic test seams rather than sleeps. Cover:
- newly created pool reports starting and zero ready workers;
- successful startup reports running and the configured worker count;
- checkout/check-in changes active and ready counts;
- a worker exit increments crash/restart counters and sets worker_crashed when no worker remains;
- a successful replacement returns readiness to ready without resetting historical counters;
- Stop reports stopping before workers are killed and stopped after cleanup.

- [ ] Step 2: Run focused tests to verify failure.

Run: go test ./internal/worker

Expected: FAIL because Pool does not expose a snapshot or lifecycle counters.

- [ ] Step 3: Implement snapshot state under the existing Pool mutex.

Initialize the state before any process is spawned. Update counts at spawn, checkout, check-in, exit, restart success/failure, Start success, and Stop. Record only sanitized categories such as startup_failed, worker_crashed, and restart_failed. Preserve the original startup error for the caller while exposing only its category through the snapshot.

- [ ] Step 4: Run the worker suite and race detector.

Run: go test ./internal/worker
Run: go test -race ./internal/worker

Expected: PASS with no data races.

- [ ] Step 5: Commit.

Run:
    git add internal/worker/pool.go internal/worker/pool_test.go internal/worker/snapshot_test.go
    git commit -m "feat: expose worker readiness snapshots"

### Task 3: Implement the versioned control HTTP server

Files:
- Create: internal/control/server.go
- Create: internal/control/auth.go
- Create: internal/control/metadata.go
- Create: internal/control/server_test.go
- Create: internal/control/auth_test.go
- Create: internal/control/metadata_test.go
- Modify: internal/metrics/metrics.go

Interfaces:
- control.NewServer(config.ControlConfig, control.SnapshotProvider, control.Metadata) (*Server, error).
- Server.Start() error and Server.Stop(context.Context) error.
- Server.Handler() http.Handler for httptest and embedding.
- control.Metadata contains only engine name/version, Go version, OS, architecture, safe worker count/timeout, capabilities, and remote-access boolean.
- The server registers GET /v1/healthz, GET /v1/readyz, GET /v1/metadata, and GET /v1/metrics.

- [ ] Step 1: Write failing handler and auth tests.

Use httptest with a fake SnapshotProvider. Assert:
- healthz returns 200 for starting, running, and stopping snapshots;
- readyz returns 200 only for a ready snapshot and 503 with a stable reason otherwise;
- metadata includes approved identity/capability fields and excludes token, executable path, environment, and arbitrary config;
- malformed or missing remote bearer tokens return 401 with no token echo;
- loopback requests do not require a token;
- metrics are served only when control is enabled and are protected on remote listeners;
- all JSON responses include version v1 and application/json.

- [ ] Step 2: Run focused tests to verify failure.

Run: go test ./internal/control

Expected: FAIL because the control server and authentication middleware do not exist.

- [ ] Step 3: Implement authentication and handlers.

Use net.ParseIP and IP.IsLoopback for the exposure rule. Use crypto/subtle.ConstantTimeCompare on byte slices after parsing exactly the Bearer scheme. Encode response structs rather than maps so field sets remain stable. Map health/readiness status to explicit HTTP codes without returning arbitrary errors.

- [ ] Step 4: Add metrics integration and run focused tests.

Keep existing metric names unchanged. Register the existing Prometheus handler on the control mux and do not register it on the public traffic mux after CLI integration. Add only bounded-label metrics needed by the current server; defer new worker lifecycle metric names to the follow-up issue.

Run: go test ./internal/control ./internal/metrics

Expected: PASS.

- [ ] Step 5: Commit.

Run:
    git add internal/control internal/metrics/metrics.go
    git commit -m "feat: add versioned local control api"

### Task 4: Integrate control lifecycle with the CLI

Files:
- Modify: internal/cli/cli.go
- Modify: internal/config/config.go
- Modify: internal/server/server.go
- Modify: README.md
- Test: internal/cli/cli_test.go
- Test: internal/server/server_test.go

Interfaces:
- runServerWithConfig validates control configuration before starting listeners.
- The control server starts before Pool.Start so child-not-started is observable.
- The pool is marked running only after successful startup; failed startup shuts down control and returns the original error.
- A single shutdown context stops the traffic server, control server, and worker pool.

- [ ] Step 1: Write failing integration tests.

Test with injected/fake server seams:
- disabled control leaves the existing public server path unchanged;
- enabled control starts on the configured local port;
- control readiness is 503 before workers start and 200 after pool startup;
- a failed worker startup closes the control listener;
- SIGTERM/shutdown closes both listeners and the pool.

- [ ] Step 2: Run focused tests to verify failure.

Run: go test ./internal/cli ./internal/server

Expected: FAIL because the CLI does not load ControlConfig or own a control server.

- [ ] Step 3: Implement lifecycle integration.

Avoid duplicating worker supervision in the control server. Start the control handler in its own goroutine, propagate listener errors through a channel, and make shutdown idempotent. Preserve existing start/dev commands and traffic address semantics.

- [ ] Step 4: Update documentation and run the full Go suite.

Document the opt-in configuration, local-only default, endpoint contracts, and remote-token warning. Run:
    go test ./...
    go vet ./...

Expected: PASS.

- [ ] Step 5: Commit.

Run:
    git add internal/cli/cli.go internal/config/config.go internal/server/server.go internal/cli/cli_test.go internal/server/server_test.go README.md
    git commit -m "feat: integrate local control lifecycle"

### Task 5: Final verification and issue handoff

Files:
- Modify: README.md
- Modify: docs/superpowers/specs/2026-10-03-control-api-design.md
- Create: docs/superpowers/plans/2026-10-03-local-control-api.md

- [ ] Step 1: Run the complete verification set.

Run:
    go test ./...
    go test -race ./...
    go vet ./...
    go build ./cmd/tusk
    git diff --check

Expected: all commands pass. Record any platform-specific limitation without presenting it as a passing check.

- [ ] Step 2: Review security and contract boundaries.

Confirm that no control response contains a token, environment value, raw path, request payload, headers, cookies, or arbitrary configuration. Confirm remote binding fails closed without a token and that readiness never reports ready with zero available workers.

- [ ] Step 3: Run a final branch review against the approved specification.

Review the net diff from origin/main, inspect lifecycle/error precedence, and fix Critical or Important findings before opening the PR.

- [ ] Step 4: Update tusk-engine issue #6 with the implementation PR and verification results.

Keep issue #6 open only if an acceptance criterion remains incomplete. Link issue #3 as the follow-up for expanded worker lifecycle metrics.

- [ ] Step 5: Commit documentation/status updates if needed.

Run:
    git add README.md docs/superpowers/specs/2026-10-03-control-api-design.md docs/superpowers/plans/2026-10-03-local-control-api.md
    git commit -m "docs: document local control api rollout"
