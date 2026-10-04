# RoadRunner Control Plane Integration Implementation Plan

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox syntax.

Goal: Make RoadRunner the actual runtime behind tusk start while the Go Engine owns configuration, process lifecycle, readiness, reload, shutdown, and the existing control API.

Architecture: Keep RoadRunner responsible for HTTP, PHP workers, Goridge IPC, pooling, recycling, and request limits. Add an injectable RoadRunner process adapter and readiness probe behind the existing runtime.Manager. The CLI starts the control API before RoadRunner, waits for RoadRunner status readiness, and never starts the legacy native server or PHP worker pool in the default path.

Tech Stack: Go 1.23, os/exec, net/http, context, Prometheus control API, RoadRunner v3 YAML projection, httptest, Go testing.

Spec: docs/superpowers/specs/2026-10-04-roadrunner-control-plane-design.md

## Global Constraints

- RoadRunner is the only supported runtime path for tusk start after this integration.
- The Engine must not reimplement RoadRunner's worker pool, IPC, request limits, or recycling.
- All RoadRunner status and RPC/reset sockets are loopback-only by default.
- No silent fallback from RoadRunner to the native runtime.
- Invalid configuration fails before a process or listener is started.
- Readiness is HTTP 200 only after RoadRunner's status plugin reports an HTTP worker ready.
- Generated runtime configuration uses a unique 0600 file under .tusk/runtime and never overwrites user-authored files.
- The Engine must not download, replace, or silently select an untrusted RoadRunner binary during tusk start.
- Control responses must not expose bearer tokens, environment values, raw paths, request data, or arbitrary configuration.
- Exact RoadRunner worker counters are not fabricated when the selected probe cannot provide them.
- Shutdown is graceful within a bounded context, with forced kill only after the deadline.

## Review Focus

- RoadRunner is missing or version-mismatched: startup fails with a diagnostic and does not fall back to native execution — covered by Task 2 and Task 4 tests.
- Status/RPC addresses are remote, malformed, or colliding with invalid ports: validation rejects them before binding — covered by Task 1 and Task 2 tests.
- RoadRunner exits before or after readiness: the runtime becomes failed/unavailable and the CLI does not leave control readiness at 200 — covered by Task 3 and Task 4 tests.
- A readiness probe receives 503, malformed JSON, transport failure, or timeout: the probe retries within the deadline and returns a stable category — covered by Task 3 tests.
- A user-authored .rr.yaml exists beside an Engine-generated file: cleanup never removes or overwrites the user file — covered by Task 2 tests.

---

### Task 1: Add runtime configuration and control snapshot contracts

Files:
- Modify: internal/config/config.go
- Test: internal/config/config_test.go
- Modify: internal/control/model.go
- Test: internal/control/model_test.go
- Modify: internal/control/server.go
- Test: internal/control/server_test.go

Interfaces:
- Produces config.RuntimeConfig with StatusAddress, RPCAddress, StartupTimeout, and ProbeInterval fields and deterministic defaults.
- Produces control.EngineFailed, process_failed and timeout readiness reasons, and RuntimeSnapshot.WorkerCountsKnown.
- Keeps control.SnapshotProvider as Snapshot() RuntimeSnapshot.
- Consumers in Tasks 2–4 use the new state and runtime fields without importing CLI or worker internals.

- [ ] Step 1: Write failing config tests.

Test exact behavior:
- Default runtime status address is 127.0.0.1:2114.
- Default runtime RPC address is tcp://127.0.0.1:6001.
- Startup timeout and probe interval are positive bounded defaults.
- Remote status or RPC addresses are rejected.
- Malformed addresses, zero/negative timing, and invalid ports return stable validation errors.

- [ ] Step 2: Write failing snapshot and handler tests.

Test exact behavior:
- A failed snapshot is not healthy and never ready.
- A stopped snapshot is not healthy and never ready.
- A runtime snapshot can report readiness while WorkerCountsKnown is false.
- Readiness responses include the stable failed/timeout reason and worker-count knowledge without leaking configuration.

- [ ] Step 3: Run focused tests to verify failure.

Run: go test ./internal/config ./internal/control

Expected: FAIL because RuntimeConfig, failed runtime states, and worker-count knowledge do not exist.

- [ ] Step 4: Implement the contracts.

Add RuntimeConfig to config.Config with loopback-safe defaults and validation. Extend control model serialization and status mapping while preserving the versioned endpoint shape and existing native-pool semantics.

- [ ] Step 5: Run focused tests to verify the contracts.

Run: go test -count=1 ./internal/config ./internal/control

Expected: PASS.

- [ ] Step 6: Commit.

git add internal/config internal/control
git commit -m "feat: define RoadRunner runtime contracts"

### Task 2: Project RoadRunner config and resolve the runtime executable

Files:
- Modify: internal/roadrunner/config.go
- Test: internal/roadrunner/config_test.go
- Modify: internal/toolchain/doctor.go
- Test: internal/toolchain/doctor_test.go
- Create: internal/runtime/config_file.go
- Test: internal/runtime/config_file_test.go

Interfaces:
- Extends roadrunner.Project(cfg *config.Config) ([]byte, error) with status and RPC/reset sections derived from cfg.Runtime.
- Produces toolchain.ResolveExecutable(root string, name ToolName) (Tool, error), or an equivalent focused resolver used by CLI.
- Produces runtime.ConfigFile with Path string and Cleanup() error.

- [ ] Step 1: Write failing RoadRunner projection tests.

Assert that rendered YAML includes version 3, the configured HTTP address, status address, RPC listen address, worker count, and request size. Assert deterministic output and rejection of remote status/RPC endpoints.

- [ ] Step 2: Write failing tool resolution tests.

Assert that a project-local RoadRunner path wins over PATH, a system RoadRunner is accepted when no project path exists, and a missing or version-mismatched binary returns a diagnostic error without installing anything.

- [ ] Step 3: Write failing generated-file lifecycle tests.

Assert that ConfigFile writes a unique 0600 file below .tusk/runtime, retains the exact projected bytes, cleans only its own generated file, and leaves an existing .rr.yaml untouched.

- [ ] Step 4: Run focused tests to verify failure.

Run: go test ./internal/roadrunner ./internal/toolchain ./internal/runtime

Expected: FAIL with missing status/RPC projection, resolver, and ConfigFile APIs.

- [ ] Step 5: Implement projection, resolution, and cleanup.

Add status and RPC structs to the RoadRunner YAML model. Resolve RoadRunner through the existing manifest/toolchain logic. Create the runtime directory with restrictive permissions, write a generated marker and YAML with 0600 permissions, and clean up only the returned path.

- [ ] Step 6: Run focused tests and commit.

Run: go test -count=1 ./internal/roadrunner ./internal/toolchain ./internal/runtime

Expected: PASS.

git add internal/roadrunner internal/toolchain internal/runtime
git commit -m "feat: project and resolve RoadRunner runtime"

### Task 3: Implement the RoadRunner process adapter and readiness probe

Files:
- Modify: internal/runtime/manager.go
- Test: internal/runtime/manager_test.go
- Create: internal/runtime/process.go
- Test: internal/runtime/process_test.go
- Create: internal/runtime/readiness.go
- Test: internal/runtime/readiness_test.go
- Create: internal/runtime/snapshot.go
- Test: internal/runtime/snapshot_test.go

Interfaces:
- Extends runtime.ProcessSpec with reload arguments and desired worker capacity.
- Adds runtime.ExecProcessFactory implementing ProcessFactory.
- Adds runtime.ReadinessProbe with Check(ctx context.Context) error.
- Adds runtime.HTTPReadinessProbe implementing ReadinessProbe.
- Adds runtime.Manager.WaitReady(ctx context.Context, probe ReadinessProbe, interval time.Duration) error.
- Adds runtime.Manager.Snapshot() control.RuntimeSnapshot.

- [ ] Step 1: Write failing fake-process manager tests.

Cover starting, readiness success, failed start, unexpected process exit, idempotent stop, graceful-stop timeout followed by kill, and reload command propagation. Assert failed state and sanitized error categories.

- [ ] Step 2: Write failing readiness-probe tests.

Use an httptest server and a fake clock or bounded interval seam to cover:
- 503 until a later 200;
- immediate 200;
- transport error followed by success;
- malformed response/status;
- context deadline;
- process manager failure while waiting.

- [ ] Step 3: Run focused tests to verify failure.

Run: go test ./internal/runtime

Expected: FAIL because process, probe, snapshot, and WaitReady behavior are not implemented.

- [ ] Step 4: Implement the process adapter.

Wrap exec.Cmd with a single wait result shared by Wait and GracefulStop. Use the project directory and explicit arguments, stream child output to the Engine logger, send the platform-supported graceful signal, and let Manager perform the bounded kill fallback. Implement reload as a one-shot reset command using the generated config and loopback RPC address.

- [ ] Step 5: Implement readiness polling and state snapshots.

Poll /ready?plugin=http with bounded timeout and interval. Mark the manager ready only after HTTP 200. Map process exits, startup failures, and probe deadlines to EngineFailed with process_failed or timeout reasons. Keep exact worker counts unknown unless a later RoadRunner-native diagnostic supplies them.

- [ ] Step 6: Run focused tests and commit.

Run: go test -count=1 ./internal/runtime

Expected: PASS.

git add internal/runtime
git commit -m "feat: supervise RoadRunner and probe readiness"

### Task 4: Replace native CLI startup with RoadRunner orchestration

Files:
- Modify: internal/cli/cli.go
- Test: internal/cli/cli_test.go
- Modify: internal/server/server.go
- Test: internal/server/server_test.go
- Modify: README.md

Interfaces:
- runServerWithConfig(cfg *config.Config) error becomes the RoadRunner orchestration entry point.
- The CLI consumes toolchain.ResolveExecutable, roadrunner.Project, runtime.ConfigFile, runtime.Manager, and runtime.HTTPReadinessProbe.
- The control server receives the runtime manager snapshot provider.
- The legacy server package is not started by tusk start and no longer owns public metrics.

- [ ] Step 1: Write failing CLI lifecycle tests.

Use fake runtime and probe seams to assert:
- invalid runtime configuration fails before control or RoadRunner start;
- missing RoadRunner fails without native fallback;
- control listener starts before the runtime process;
- readiness remains unavailable until the probe succeeds;
- process failure stops control and returns a non-zero error;
- SIGTERM stops RoadRunner and control under one shutdown context.

- [ ] Step 2: Write failing documentation/traffic tests.

Assert that the public server path is not invoked by the start flow and that the README describes RoadRunner as the only start runtime, including status/readiness configuration.

- [ ] Step 3: Run focused tests to verify failure.

Run: go test ./internal/cli ./internal/server

Expected: FAIL because the CLI still constructs the native worker pool/server.

- [ ] Step 4: Implement the lifecycle switch.

Validate config, resolve RoadRunner, render the generated config, construct the manager/probe, start control, start RoadRunner, await readiness, wait for signals or runtime failure, then stop runtime and control. Remove native worker/server construction from this path without deleting migration-era packages.

- [ ] Step 5: Update documentation.

Document the RoadRunner-only start path, status/RPC loopback defaults, startup failure behavior, and the absence of native fallback. Keep Composer and toolchain diagnostics aligned with the actual lifecycle.

- [ ] Step 6: Run focused tests and commit.

Run: go test -count=1 ./internal/cli ./internal/server

Expected: PASS.

git add internal/cli internal/server README.md
git commit -m "feat: run applications under RoadRunner control"

### Task 5: Full verification and issue handoff

Files:
- Modify: docs/superpowers/specs/2026-10-04-roadrunner-control-plane-design.md
- Modify: README.md

Interfaces:
- The completed branch satisfies the approved specification and is ready for a PR against the synchronized main.

- [ ] Step 1: Run the complete verification set.

Run:
- go test -count=1 ./...
- go test -race ./...
- go vet ./...
- go build ./cmd/tusk
- git diff --check

Expected: all commands pass.

- [ ] Step 2: Review lifecycle and security boundaries.

Confirm no native server or PHP pool is started by tusk start, no remote runtime control socket is generated by default, no generated config cleanup can remove user files, and failed/readiness states are exposed without raw child output.

- [ ] Step 3: Review the net diff against main and fix Critical or Important findings.

Inspect process ownership, shutdown idempotence, probe timeout behavior, error precedence, and Windows signal handling before publishing.

- [ ] Step 4: Update issue #5 with the implementation PR, verification results, and any follow-up issue links.

Keep issue #5 open only if an acceptance criterion remains incomplete. Link the observability and native cleanup follow-ups rather than expanding this integration.

- [ ] Step 5: Commit final documentation/status changes.

git add README.md docs/superpowers/specs/2026-10-04-roadrunner-control-plane-design.md
git commit -m "docs: finalize RoadRunner control plane rollout"
