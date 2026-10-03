# Tusk Engine Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tornar o servidor Go confiável para workers PHP persistentes sem alterar o protocolo NDJSON ou os comandos públicos.

**Architecture:** Manter o servidor HTTP e o pool existentes, mas introduzir limites explícitos na fronteira HTTP, uma interface mínima de worker handler para testes e um estado protegido para leases do pool. Configuração e Docker serão corrigidos sem reescrever a CLI.

**Tech Stack:** Go 1.23+, `net/http`, `httptest`, Prometheus client, Docker, PHP worker NDJSON.

**Spec:** `docs/superpowers/specs/2026-10-03-production-hardening-design.md`

## Global Constraints

- `max_body_bytes` default 10 MiB.
- `max_upload_bytes` default 10 MiB por arquivo.
- `max_upload_files` default 20.
- Requests acima dos limites retornam HTTP 413.
- Scripts de `tusk.json` vencem conflitos com `composer.json`.
- O worker padrão da imagem Docker fica em `/app/worker.php`.
- NDJSON permanece o contrato entre engine e worker.

## Review Focus

- `/public/../secret` nunca pode escapar do diretório público — Task 2.
- Um body maior que o limite não pode ser totalmente lido em memória — Task 2.
- Um worker morto após timeout não pode voltar à fila — Task 3.
- Shutdown não pode iniciar novos workers — Task 3.
- Scripts presentes nos dois arquivos de configuração precisam ser mesclados com precedência documentada — Task 1.

### Task 1: Configuration model and deterministic loading

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `tusk.json` only if defaults need documenting
- Modify: `README.md` for exact limits and precedence

**Interfaces:**
- Add `MaxBodyBytes int64`, `MaxUploadBytes int64`, and `MaxUploadFiles int` to `Config` with JSON names `max_body_bytes`, `max_upload_bytes`, and `max_upload_files`.
- Keep `LoadConfig()` as the public entry point and add an internal root-aware loader for tests and relative-path resolution.

- [ ] **Step 1: Write failing tests** for defaults, tusk-over-composer field precedence, script merging, and relative project-root resolution.
- [ ] **Step 2: Run** `go test ./internal/config -run 'Test' -v`; confirm the current scripts map is replaced instead of merged and limit fields are absent.
- [ ] **Step 3: Implement the config loader** with deterministic merge order and validated positive limits; reject invalid configuration with a clear error or safe defaults.
- [ ] **Step 4: Run the focused tests** and confirm all pass.
- [ ] **Step 5: Commit** `fix: make engine configuration deterministic`.

### Task 2: Safe HTTP boundary and request limits

**Files:**
- Modify: `internal/server/server.go`
- Create: `internal/server/worker_handler.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Introduce an internal `WorkerHandler` interface with `HandleRequest(map[string]interface{}) (map[string]interface{}, error)` so HTTP tests do not require PHP.
- `Server` accepts `WorkerHandler` while `worker.Pool` remains an implementation.

- [ ] **Step 1: Write failing tests** for valid static files, traversal attempts, body over `max_body_bytes`, too many uploads, oversized uploads, and the 413 response.
- [ ] **Step 2: Run** `go test ./internal/server -run 'Test' -v`; confirm traversal and unbounded body behavior fail the new expectations.
- [ ] **Step 3: Implement canonical path confinement, `http.MaxBytesReader`, content-length checks, multipart file/count validation, and controlled 413 responses.** Preserve query/header/cookie forwarding.
- [ ] **Step 4: Run focused server tests and `go test ./...`; confirm normal requests and metrics still work.
- [ ] **Step 5: Commit** `fix: enforce HTTP path and payload boundaries`.

### Task 3: Worker pool lease lifecycle

**Files:**
- Modify: `internal/worker/pool.go`
- Modify: `internal/worker/pool_test.go`
- Create: `internal/worker/test_helper_test.go`

**Interfaces:**
- Keep `NewPool`, `Start`, `HandleRequest`, and `Stop` signatures.
- Add internal process state/lease helpers and a test-only command factory so tests can use the Go test binary as an NDJSON worker without PHP.

- [ ] **Step 1: Write failing tests** for timeout not requeueing a killed worker, idle-worker exit/restart, shutdown preventing restart, restart failure visibility, and concurrent lease uniqueness.
- [ ] **Step 2: Run** `go test -race ./internal/worker -run 'Test' -v`; confirm the current pool can requeue killed workers and depends on system PHP.
- [ ] **Step 3: Implement protected worker state, queue removal before restart, cancellation checks after backoff, bounded request goroutines, and cleanup on partial `Start` failure.** Ensure one lease per worker.
- [ ] **Step 4: Run worker tests with `-race` and the full Go suite; confirm no race reports and no PHP dependency in unit tests.**
- [ ] **Step 5: Commit** `fix: make worker leases lifecycle-safe`.

### Task 4: Docker and runtime packaging

**Files:**
- Modify: `Dockerfile`
- Modify: `.goreleaser.yaml` only if archive naming differs from README
- Create: `internal/packaging/dockerfile_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write a failing packaging check** asserting the default worker exists at `/app/worker.php` and the container configuration matches engine defaults.
- [ ] **Step 2: Run the check** and confirm the current image copies the worker to `/usr/local/bin` instead.
- [ ] **Step 3: Copy the worker to `/app/worker.php`, keep project-root defaults aligned, and add a Docker smoke-test command to the documentation/CI path.
- [ ] **Step 4: Run the packaging check, `go build ./...`, and `go vet ./...`.
- [ ] **Step 5: Commit** `fix: align Docker worker packaging with defaults`.

### Task 5: Full verification and issue handoff

**Files:**
- Modify: `README.md` for operational limits and shutdown behavior
- Modify: `README.md` for deferred features and operational limits

- [ ] **Step 1: Run** `go test ./...`.
- [ ] **Step 2: Run** `go test -race ./...`.
- [ ] **Step 3: Run** `go vet ./...` and `go build ./...`.
- [ ] **Step 4: Review the complete diff and verify the NDJSON contract field-by-field.**
- [ ] **Step 5: Create public GitHub issues only for intentionally deferred features, with acceptance criteria.
- [ ] **Step 6: Commit** `docs: document production hardening status` if README changed; otherwise record the clean verification without creating an empty commit.
