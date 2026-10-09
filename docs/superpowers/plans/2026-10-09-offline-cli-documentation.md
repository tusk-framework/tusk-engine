# Offline CLI Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship complete version-matched user guides inside the Tusk Engine binary and make them readable offline through `tusk docs`.

**Architecture:** Keep canonical user-facing Markdown in `docs/user-guide/` and expose only that directory through a Go `embed.FS` package in `docs/`. `tusk docs` lists the embedded topics or prints one selected guide; it never accesses the network and adds no runtime dependency or pager requirement.

**Tech Stack:** Go 1.23 standard library, `embed`, Markdown source, GoReleaser version linker flag.

**Spec:** `docs/superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md`

## Global Constraints

- Documentation must work offline without a browser, PHP, or Composer.
- `docs/user-guide/` Markdown is the sole source of truth; do not keep manually edited copies.
- Keep `tusk help` concise and point to `tusk docs`.
- `tusk start`, ordinary `doctor`, and `tusk docs` do not perform network access.
- Identify the Engine/documentation version shown by the binary.

## Review Focus

- Unknown topic or path traversal: reject it and list valid topics; test `../` and absolute paths.
- Embedded files absent in development or release builds: fail tests/build contract when an expected page is missing.
- Docs content/version mismatch: verify runtime version output and every embedded index entry.
- Large/unexpected files are embedded: restrict the embed glob to user guides; test internal specs/plans are excluded.
- Terminal output/pipe failure: return write errors and avoid panics; test a failing writer.

---

### Task 1: Create the canonical guide set and embed only user content

**Files:**
- Create: `docs/user-guide/index.md` — topic index and quick start.
- Create: `docs/user-guide/toolchain.md` — profiles, pinning, setup, cache, and offline use.
- Create: `docs/user-guide/php-extensions.md` — profiles, platform checks, host prerequisites, and extension troubleshooting.
- Create: `docs/user-guide/composer.md` — commands, PHAR/PHP selection, lockfile ownership, and real platform checks.
- Create: `docs/user-guide/runtime.md` — bootstrap, RoadRunner, worker lifecycle, start/stop, and production deployment.
- Create: `docs/user-guide/security.md` — signed catalog, artifact verification, trust anchors, and safe updates.
- Create: `docs/user-guide/ci-docker.md` — pinned CI and Docker profiles.
- Create: `docs/user-guide/upgrading.md` — toolchain and documentation version upgrades/migration.
- Create: `docs/embed.go` — `//go:embed user-guide/*.md` read-only filesystem.
- Modify: `docs/guides/README.md`, `README.md` — link repository readers to the canonical guides.
- Test: `docs/embed_test.go` — ensure every indexed topic exists and no design plans/specs are embedded.

**Interfaces:**
- Produces: `docs.UserGuide embed.FS` with stable paths `user-guide/<topic>.md` and an index document.

- [ ] **Step 1: Write failing tests** for index availability, all expected guide topics, and exclusion of `superpowers` specs/plans.
- [ ] **Step 2: Run** `go test ./docs -count=1`; verify failures identify missing files.
- [ ] **Step 3: Write the guides** from the current CLI/runtime contracts, explicitly marking features not yet implemented instead of documenting future behavior as current.
- [ ] **Step 4: Add the package-local embed FS** and repository links to the same files.
- [ ] **Step 5: Run** `go test ./docs -count=1`; expect all embedded content checks to pass.
- [ ] **Step 6: Commit** `docs: add canonical offline user guide content`.

### Task 2: Add `tusk docs` topic listing and reader

**Files:**
- Create: `internal/cli/docs.go` — topic normalization, listing, rendering-as-Markdown text, and version header.
- Create: `internal/cli/docs_test.go` — listing, topic, invalid-input, offline, and writer tests.
- Modify: `internal/cli/cli.go` — dispatch `docs` before script and PHP-command fallback; add concise help entries.
- Modify: `.goreleaser.yaml` — inject the release version into an `internal/cli` version variable.

**Interfaces:**
- Produces: `runDocs(args []string, output io.Writer, files fs.FS, version string) error` supporting no-argument index output and exactly one `<topic>` argument.
- Unknown topics return a helpful error plus the embedded index; names are restricted to known basenames and cannot escape the guide directory.

- [ ] **Step 1: Write failing tests** for index, topic read, unknown topic, too many args, traversal, empty content, failing writer, version header, and network-free operation.
- [ ] **Step 2: Run** `go test ./internal/cli -run TestDocs -count=1`; verify the new tests fail.
- [ ] **Step 3: Implement** the dispatcher and reader using only the embedded filesystem and standard library; preserve Markdown links/code blocks in readable terminal output.
- [ ] **Step 4: Inject release version** through GoReleaser while retaining a deterministic `dev` value for local builds.
- [ ] **Step 5: Run** `go test ./internal/cli ./docs -count=1` and `go build ./cmd/tusk`; expect both to pass.
- [ ] **Step 6: Commit** `feat(cli): provide offline versioned documentation`.

### Task 3: Verify the built artifact contains the same guides

**Files:**
- Modify: `internal/packaging/documentation_contract_test.go` — assert the CLI command/help and canonical source links.
- Modify: `.github/workflows/test.yml` — build the binary and assert `tusk docs` plus representative topics work with network disabled.
- Test: `internal/cli/docs_test.go`, packaging tests.

**Interfaces:**
- Consumes: Task 1's embedded guide contract and Task 2's CLI contract.
- Produces: release/CI evidence that the distributed Engine binary can read all canonical guides offline.

- [ ] **Step 1: Write a failing packaging smoke assertion** that runs the built Engine's docs index and a topic with network disabled.
- [ ] **Step 2: Run** `go test ./internal/packaging -run TestOfflineDocs -count=1`; verify failure before workflow integration.
- [ ] **Step 3: Add the build smoke** without any PHP/Composer prerequisite or network access to the docs command.
- [ ] **Step 4: Run** `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`; expect all commands to pass.
- [ ] **Step 5: Commit** `test: verify offline docs in Engine artifacts`.

## Execution Order

Tasks 1 and 2 are sequential; Task 3 depends on both. This plan is independent of PHP bundle publishing and can be implemented separately after review.
