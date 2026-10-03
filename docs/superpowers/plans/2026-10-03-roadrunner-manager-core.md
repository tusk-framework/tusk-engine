# RoadRunner Manager Core

## Goal

Introduce the first testable control-plane core in `tusk-engine` without placing the Engine in the HTTP request path or duplicating RoadRunner's worker pool.

## Scope

1. Define canonical RoadRunner settings derived from the current project configuration.
2. Render a deterministic `.rr.yaml` representation.
3. Define explicit runtime states and legal transitions.
4. Add a process abstraction for start, wait, graceful stop, forced stop, and reload.
5. Cover the state machine and command/configuration projection with unit tests.

The CLI will not switch its production start path in this slice until the manager has readiness polling and integration coverage with a real RoadRunner binary.

## TDD tasks

### Task 1 — Canonical RoadRunner projection

- Add failing tests for default address, worker command, worker count, limits, and deterministic YAML.
- Implement the projection and validation.

### Task 2 — Runtime state machine

- Add failing tests for `created`, `starting`, `ready`, `stopping`, `stopped`, and `failed` transitions.
- Reject illegal transitions with actionable errors.

### Task 3 — Process abstraction

- Add a fake process for lifecycle tests.
- Implement manager start/stop/reload behavior without owning PHP workers.
- Keep OS-specific process signaling behind the abstraction.

## Follow-up

- readiness polling against RoadRunner;
- real child process launcher and log capture;
- `tusk start` integration;
- health/metadata API;
- integration tests with PHP and RoadRunner in CI.

## Verification

- `go test ./...`;
- `go vet ./...`;
- `go build ./...`;
- race tests where the local toolchain supports CGO;
- `git diff --check`.
