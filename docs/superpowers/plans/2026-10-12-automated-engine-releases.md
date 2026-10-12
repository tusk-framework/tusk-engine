# Automated Engine Releases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically calculate, tag, sign, attest, and publish Engine releases after all required main-branch validation succeeds.

**Architecture:** Use a pinned, maintained Conventional Commit parser as a repository-local version calculator with deterministic tests. A serialized `workflow_run` aggregator verifies successful Go, Docker validation, and skeleton smoke runs for the exact same main SHA before calculating a version. Refactor the existing release workflow into a shared exact-tag publication path called as a reusable workflow job or composite action; create an immutable tag only after confirming the validated SHA is still main. Keep mutable Docker `main` image publication distinct from release eligibility and publish versioned images only for validated release commits.

**Tech Stack:** Go 1.23, GitHub Actions, GoReleaser v2, Docker Buildx, GitHub `GITHUB_TOKEN`, existing catalog signer and GitHub artifact attestations.

**Spec:** `docs/superpowers/specs/2026-10-12-automated-engine-releases-design.md`

## Global Constraints

- Release increments: `feat` minor; `fix`/`perf` patch; valid `BREAKING CHANGE:` or `!` major; other types do not release independently.
- A release requires successful Go build/test/vet, Docker build validation, and Skeleton smoke for the exact source commit.
- No release PR, PAT, or new long-lived credential is introduced.
- Never move or overwrite an existing version tag; do not publish a stale commit after `main` advances.
- Preserve official catalog validation, signing, verification, attestations, digest binding, checksums, binaries, and provenance.
- Use a pinned version-calculation dependency/action; do not consume an unpinned `latest` tool.
- Keep `.tmp/` and all unrelated user changes untouched.

## Review Focus

- Squash-merge title and resulting commit subject differ — test the actual GitHub squash metadata contract and fail closed if release information is lost.
- Breaking notation, malformed footers, and mixed release levels — pin parser results and precedence using table-driven Go tests.
- A queued workflow observes a newer `main` or a competing manual tag — test stale-SHA and existing-tag decisions before mutation.
- Failure occurs after tag creation but before GoReleaser completes — test retry behavior that keeps the tag immutable and avoids duplicate assets/releases.
- Docker validation succeeds while mutable image publication fails (or vice versa) — test that release gating depends on validation, not the mutable image side effect.

---

### Task 1: Deterministic semantic-version calculator

**Files:**
- Create: `internal/release/version.go`
- Create: `internal/release/version_test.go`
- Modify: `go.mod`

**Interfaces:**
- Produces: `CalculateNextVersion(current string, messages []string) (version string, shouldRelease bool, err error)`; output version omits the `v` tag prefix.

- [ ] **Step 1: Write table-driven tests** for patch/minor/major precedence, no-release-only messages, empty range, invalid current tag, malformed Conventional Commit messages, valid breaking footer, `!` notation, and mixed release levels.
- [ ] **Step 2: Run `go test ./internal/release`** and confirm it fails because the calculator is not implemented.
- [ ] **Step 3: Add a pinned maintained Go Conventional Commit parser dependency** and implement `CalculateNextVersion` using the policy in the spec; reject malformed release metadata and normalize a stable `vMAJOR.MINOR.PATCH` input.
- [ ] **Step 4: Run `go test ./internal/release`** and confirm all policy cases pass.
- [ ] **Step 5: Run `go test ./...` and `go vet ./...`** to check integration and dependency compatibility.
- [ ] **Step 6: Commit** as `feat: calculate semantic engine release versions`.

### Task 2: Version calculation CLI and history-range contract

**Files:**
- Create: `cmd/tusk-release/main.go`
- Create: `cmd/tusk-release/main_test.go`
- Modify: `CONTRIBUTING.md`

**Interfaces:**
- Consumes: `internal/release.CalculateNextVersion(current string, messages []string)`.
- Produces: a CI-friendly command accepting the previous tag and commit-message input, emitting exactly `version=<value>` and `should_release=true|false`; malformed input exits nonzero without partial output.

- [ ] **Step 1: Add CLI tests** for valid release, no-release, malformed input, and clean output format.
- [ ] **Step 2: Run `go test ./cmd/tusk-release`** and confirm the tests fail before implementation.
- [ ] **Step 3: Implement the CLI** with explicit flags for previous stable tag and target commit; obtain messages using read-only `git log --no-merges --format=%B <tag>..<target>`; ensure it does not fetch network data or mutate Git state.
- [ ] **Step 4: Clarify `CONTRIBUTING.md`** that PR titles are validated as Conventional Commits and the squash commit subject must preserve the title; specify breaking footer/`!` syntax.
- [ ] **Step 5: Run `go test ./cmd/tusk-release ./internal/release`** and inspect output for absence of logs that could corrupt GitHub outputs.
- [ ] **Step 6: Commit** as `feat: expose deterministic release calculation to CI`.

### Task 3: Required validation jobs and PR title validation

**Files:**
- Create: `.github/workflows/pr-title.yml`
- Modify: `.github/workflows/go.yml`
- Modify: `.github/workflows/test.yml`
- Modify: `.github/workflows/docker-publish.yml`

**Interfaces:**
- Produces: stable workflow names and success conclusions consumed by the `workflow_run` aggregator; PR title validation blocks invalid Conventional Commit titles before merge.

- [ ] **Step 1: Define workflow validation cases** for valid `feat`, `fix`, `perf`, arbitrary non-release type, `!` breaking notation, footer breaking notation, and invalid/malformed title; use the same parser/policy as Task 1 where practical.
- [ ] **Step 2: Run local action/workflow lint and the Go tests** to establish expected validation before editing workflow behavior.
- [ ] **Step 3: Add a title-validation workflow** triggered on pull request opened/edited/synchronize/reopened; pin its action or use the Task 1 parser in a minimal Go invocation.
- [ ] **Step 4: Split Docker build validation from registry publication** so the required Docker validation job is read-only and deterministic; keep mutable `main` push separate and non-gating.
- [ ] **Step 5: Give the Go and skeleton workflows stable check names** and ensure each reports failure (not silent success) when prerequisites or tests fail.
- [ ] **Step 6: Verify workflow syntax and run relevant tests**; document the required check names and title contract.
- [ ] **Step 7: Commit** as `ci: validate conventional pull request titles and release gates`.

### Task 4: Shared exact-tag signed publication workflow

**Files:**
- Modify: `.github/workflows/release.yml`
- Create: `.github/workflows/publish-release.yml` (reusable workflow)
- Modify: `.goreleaser.yaml`

**Interfaces:**
- Produces: shared publication implementation accepting an immutable version tag and expected source SHA, validating both, then running the existing signed catalog and GoReleaser sequence exactly once. If implemented as a reusable workflow, it is invoked as a job, never as a step.
- Consumes: tag/version and SHA from either the automatic main orchestrator or an explicitly supported manual stable-tag invocation.

- [ ] **Step 1: Capture existing release contract** in checks for signed catalog, provenance, checksums, attestations, and exact binary version/digest.
- [ ] **Step 2: Run current release validation tests/checks** to establish the baseline and inspect GoReleaser config behavior for exact tags.
- [ ] **Step 3: Extract the current signed release steps** into a reusable workflow with least-privilege permissions, explicit tool versions, stable-tag validation, and expected-SHA verification.
- [ ] **Step 4: Keep tag-triggered compatibility/manual publishing** as a thin caller of the reusable workflow, rejecting non-stable tags and never moving tags.
- [ ] **Step 5: Add workflow-level validation** that missing signing inputs, invalid payload, mismatched SHA, or wrong tag fail before publication.
- [ ] **Step 6: Validate workflow YAML and GoReleaser configuration**; confirm existing release assets and trust chain are preserved.
- [ ] **Step 7: Commit** as `refactor: share exact-tag signed release publication`.

### Task 5: Serialized automatic main release orchestrator

**Files:**
- Create: `.github/workflows/automatic-release.yml`
- Modify: `.github/workflows/docker-publish.yml`
- Modify: `.github/workflows/publish-release.yml`

**Interfaces:**
- Consumes: `workflow_run` events and API results proving successful required Go, Docker validation, and Skeleton smoke runs for one immutable main SHA; Task 2 CLI output; Task 4 shared publisher.
- Produces: at most one immutable `vMAJOR.MINOR.PATCH` tag and matching GitHub release for that validated SHA; versioned Docker image only for a published release commit.

- [ ] **Step 1: Add orchestration contract tests/scripts** for no release, successful release, `main` advancing while queued, tag collision, already-complete release, and retry after partial publication.
- [ ] **Step 2: Run those tests** and confirm the stale-SHA/tag-race cases fail before workflow implementation.
- [ ] **Step 3: Add a `workflow_run` aggregator** for Go, Docker validation, and Skeleton smoke. Accept only same-repository `push` runs on `main`; query all required workflow runs for the exact head SHA and proceed only when each has a successful conclusion. Serialize release runs (`cancel-in-progress: false`), keep calculation read-only, and scope mutation/publication to a job with `contents: write`.
- [ ] **Step 4: Before tag mutation, re-fetch remote tags and `main`;** skip stale runs if SHA changed, skip cleanly when no release is due, and fail on conflicting target tags.
- [ ] **Step 5: Create an annotated version tag at the exact tested SHA** and call the shared publisher within the same workflow run, without relying on tag push to trigger Actions.
- [ ] **Step 6: Implement retry rules**: same tag + same SHA + incomplete release can resume idempotently; completed releases exit successfully; different SHA or conflicting assets fail without mutation.
- [ ] **Step 7: Publish Docker image tag `v<version>` from the same validated SHA** after version calculation; preserve mutable `main` image behavior separately and prevent it from satisfying validation dependencies.
- [ ] **Step 8: Validate action graph/YAML and run all Go tests**; assert each acceptance criterion has a corresponding workflow/test check.
- [ ] **Step 9: Commit** as `feat: publish engine releases after successful main checks`.

### Task 6: Documentation and end-to-end release verification

**Files:**
- Modify: `README.md`
- Modify: `CONTRIBUTING.md`
- Modify: `docs/user-guide/releasing.md`
- Create: `docs/user-guide/release-automation.md`

**Interfaces:**
- Produces: operator documentation for version calculation, skipped release conditions, immutable tag behavior, failed publication recovery, credential requirements, and Docker tag behavior.

- [ ] **Step 1: Add documentation checks** for every policy and recovery case in the spec, including that no PAT is needed and secrets remain unprinted.
- [ ] **Step 2: Run documentation tests/link checks** and verify they fail if release policy/recovery sections are absent.
- [ ] **Step 3: Document automatic release behavior** and update README/manual release guidance to distinguish normal automatic releases from explicit stable-tag recovery.
- [ ] **Step 4: Review workflows and docs together** against all 13 acceptance criteria in the spec; record any criterion that cannot be exercised without GitHub-hosted integration as a manual validation step.
- [ ] **Step 5: Run `go test ./...`, `go vet ./...`, workflow YAML lint, and `goreleaser check`** with pinned/local tooling as available.
- [ ] **Step 6: Commit** as `docs: explain automated engine release lifecycle`.

## Execution Notes

- Update local `main` from `origin/main` and verify the clean baseline before creating a branch; never use `codex/` as a branch prefix.
- Execute in an isolated worktree and use a pull request for all changes.
- Do not merge or create a production release until the PR is green and the release workflow has been reviewed for token permissions and tag immutability.
- The repository already has an untracked `.tmp/` directory; preserve it throughout execution.
