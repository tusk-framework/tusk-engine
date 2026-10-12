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
- [ ] **Step 4: Clarify `CONTRIBUTING.md`** that PR titles are validated as Conventional Commits; squash commits preserve the validated title, while regular merge commits require Conventional Commit messages for each included non-merge commit and the calculator ignores the synthetic merge wrapper; specify breaking footer/`!` syntax.
- [ ] **Step 5: Run `go test ./cmd/tusk-release ./internal/release`** and inspect output for absence of logs that could corrupt GitHub outputs.
- [ ] **Step 6: Commit** as `feat: expose deterministic release calculation to CI`.

### Task 3: Required validation jobs and PR title validation

**Files:**
- Create: `.github/workflows/pr-title.yml`
- Create: `.github/workflows/docker-validation.yml`
- Modify: `.github/workflows/go.yml`
- Modify: `.github/workflows/test.yml`
- Modify: `.github/workflows/docker-publish.yml`
- Modify: `internal/release/version.go`
- Modify: `internal/release/version_test.go`
- Modify: `cmd/tusk-release/main.go`
- Modify: `cmd/tusk-release/main_test.go`

**Interfaces:**
- Produces: stable workflow names (`Go`, `Docker validation`, `Skeleton smoke`) and success conclusions consumed by the `workflow_run` aggregator; PR title validation uses the same Go parser as release calculation and blocks invalid Conventional Commit titles before merge.
- Produces: `release.ValidateConventionalCommit(message string) error` and `tusk-release validate-message --message <title>`; valid non-release types are accepted, malformed titles fail.

- [ ] **Step 1: Add parser/CLI tests** for valid `feat`, `fix`, `perf`, arbitrary non-release type, `!` notation, breaking footer, malformed title, and invalid UTF-8 where relevant.
- [ ] **Step 2: Run focused Go tests** and confirm new validation tests fail before implementation.
- [ ] **Step 3: Implement `release.ValidateConventionalCommit`** with the pinned parser and expose `tusk-release validate-message --message <title>`; add CLI and package tests.
- [ ] **Step 4: Add `pr-title.yml`** triggered on opened/edited/synchronize/reopened pull requests. Pass title through an environment variable (never interpolate untrusted title into a shell script) and validate with the Go CLI; grant read-only permissions.
- [ ] **Step 5: Split Docker validation from image publication**: create a read-only `Docker validation` workflow and make `docker-publish.yml` callable only as a reusable workflow (no direct `push` trigger); Task 5 calls it only after the aggregator verifies every required check, so image publication cannot run on an unvalidated main commit or affect the validation workflow conclusion.
- [ ] **Step 6: Set stable workflow names** `Go`, `Docker validation`, and `Skeleton smoke`; make missing smoke prerequisites fail rather than silently skip required checks.
- [ ] **Step 7: Verify workflow syntax and Go tests**; document required check names and the title/mixed-merge contract.
- [ ] **Step 8: Commit** as `ci: validate conventional pull request titles and release gates`.

### Task 4: Shared exact-tag signed publication workflow

**Files:**
- Modify: `.github/workflows/release.yml`
- Create: `.github/actions/publish-release/action.yml` (local composite action)
- Modify: `.goreleaser.yaml`

**Interfaces:**
- Produces: shared composite publication action accepting an immutable version tag and expected source SHA, validating both, then running the existing signed catalog and GoReleaser sequence exactly once.
- Consumes: tag/version and SHA from either the automatic main orchestrator or an explicitly supported manual stable-tag invocation; signing credentials are passed only through caller step environment and never as logged inputs.

- [ ] **Step 1: Capture existing release contract** in checks for signed catalog, provenance, checksums, attestations, and exact binary version/digest.
- [ ] **Step 2: Run current release validation tests/checks** to establish the baseline and inspect GoReleaser config behavior for exact tags.
- [ ] **Step 3: Extract the current signed release steps** into a local composite action with stable-tag validation, expected-SHA verification, Go 1.23, and pinned GoReleaser CLI v2.18.3/action v7.2.3 (`f06c13b6b1a9625abc9e6e439d9c05a8f2190e94`).
- [ ] **Step 4: Configure safe GoReleaser retry behavior** with `preflight.fail_on_error: true`, `use_existing_draft: true`, and `replace_existing_artifacts: true`; retries may reuse an incomplete draft for the same tag/SHA, but must not alter a published release.
- [ ] **Step 5: Keep tag-triggered compatibility/manual publishing** as a thin caller of the composite action, rejecting non-stable tags and never moving tags; scope token/OIDC/attestation permissions to the caller's publication job.
- [ ] **Step 6: Add workflow-level validation** that missing signing inputs, invalid payload, mismatched SHA, or wrong tag fail before publication.
- [ ] **Step 7: Validate workflow YAML and GoReleaser configuration**; confirm existing release assets and trust chain are preserved.
- [ ] **Step 8: Commit** as `refactor: share exact-tag signed release publication`.

### Task 5: Serialized automatic main release orchestrator

**Files:**
- Create: `.github/workflows/automatic-release.yml`
- Modify: `.github/workflows/docker-publish.yml`
- Modify: `.github/workflows/publish-release.yml`

**Interfaces:**
- Consumes: `workflow_run` events and API results proving successful required Go, Docker validation, and Skeleton smoke runs for one immutable main SHA; Task 2 CLI output; Task 4 local composite publisher.
- Produces: at most one immutable `vMAJOR.MINOR.PATCH` tag and matching GitHub release for that validated SHA; mutable `main` image and optional versioned Docker image only after all required checks succeed for the same SHA.

- [ ] **Step 1: Add orchestration contract tests/scripts** for no release, successful release, `main` advancing while queued, tag collision, already-complete release, and retry after partial publication.
- [ ] **Step 2: Run those tests** and confirm the stale-SHA/tag-race cases fail before workflow implementation.
- [ ] **Step 3: Add a `workflow_run` aggregator** for Go, Docker validation, and Skeleton smoke. Accept only same-repository `push` runs on `main`; query all required workflow runs for the exact head SHA and proceed only when each has a successful conclusion. Serialize release runs (`cancel-in-progress: false`), keep calculation read-only, and scope mutation/publication to a job with `contents: write`.
- [ ] **Step 4: Before tag mutation, re-fetch remote tags and `main`;** skip stale runs if SHA changed, skip cleanly when no release is due, and fail on conflicting target tags.
- [ ] **Step 5: Create an annotated version tag at the exact tested SHA** and call the shared publisher within the same workflow run, without relying on tag push to trigger Actions.
- [ ] **Step 6: Implement retry rules**: same tag + same SHA + incomplete release can resume idempotently; completed releases exit successfully; different SHA or conflicting assets fail without mutation.
- [ ] **Step 7: Publish Docker image tag `v<version>` from the same validated SHA** after version calculation; preserve mutable `main` image behavior separately and prevent it from satisfying validation dependencies.
- [ ] **Step 8: Call the reusable Docker publisher only after all required validation succeeds**, publishing `main` even when semantic version calculation reports no release, and publishing both `main` and `v<version>` when a release is due.
- [ ] **Step 9: Validate action graph/YAML and run all Go tests**; assert each acceptance criterion has a corresponding workflow/test check.
- [ ] **Step 10: Commit** as `feat: publish engine releases after successful main checks`.

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
