# Task 3 implementation report

## Status

Implemented Task 3 in the automated-engine-releases worktree. The change is limited to the eight files requiring content changes from the nine-file brief; `.github/workflows/go.yml` was inspected and left unchanged because it already declares the exact stable workflow name `Go`.

## Changes

- Added `release.ValidateConventionalCommit(message string) error`, using the pinned `go-conventionalcommits` v0.13.0 parser already shared by release calculation. It rejects malformed messages, invalid UTF-8, and empty `BREAKING CHANGE:` footers while accepting valid release and non-release types and breaking notation.
- Added `tusk-release validate-message --message <title>` and package/CLI table tests for `feat`, `fix`, `perf`, arbitrary types, `!`, breaking footers, malformed headers, and invalid UTF-8.
- Added the `PR title` workflow for opened, edited, synchronize, and reopened events. The title is passed using the `PR_TITLE` environment variable, then quoted as a shell variable; workflow permissions are read-only (`contents: read`).
- Added the stable `Docker validation` workflow with `push: false` and read-only repository permissions.
- Changed `docker-publish.yml` into a separate mutable `main` image publication workflow with only the package write permission required for publication. PRs and tags do not run this publisher.
- Kept the existing `Skeleton smoke` stable workflow name and changed absent prerequisite handling from a successful skip to an explicit job failure.

## Merge and release contract

PR titles must be valid Conventional Commit messages for every merge strategy. For squash merges, the validated title is the canonical release signal and the squash commit subject must preserve it. For regular merge commits, the title is validated before merge and individual non-merge commits are classified; synthetic merge wrapper messages are ignored by release calculation. The required main-branch validation workflow names are exactly `Go`, `Docker validation`, and `Skeleton smoke`; mutable image publication is not a release validation gate.

## Verification evidence

- TDD red run before implementation: `go test ./internal/release ./cmd/tusk-release` failed because `ValidateConventionalCommit` was undefined and `validate-message` was not recognized by the CLI.
- Focused post-implementation run: `go test ./internal/release ./cmd/tusk-release` passed.
- Full suite: `go test ./...` passed.
- Static analysis: `go vet ./...` passed.
- Workflow syntax: all five workflow files (`go.yml`, `test.yml`, `docker-publish.yml`, `docker-validation.yml`, and `pr-title.yml`) parsed successfully with the repository-pinned `go.yaml.in/yaml/v2` v2.4.2 library.
- `git diff --check` passed. `actionlint` was not installed in the environment, so GitHub-specific linting was unavailable; workflow event, permission, environment interpolation, job naming, and build/publish configuration were manually reviewed.

## Commit

Required commit message: `ci: validate conventional pull request titles and release gates`

## Round 1 review fix — reusable Docker publisher

Changed `.github/workflows/docker-publish.yml` to expose only the `workflow_call` trigger. The reusable interface requires `image-name` (fully qualified image name) and accepts optional newline-separated `tags` (default empty); the existing mutable `main` tag is always included. The caller can supply a version tag from Task 5 after it verifies Go, Docker validation, and Skeleton smoke succeeded for the exact SHA. `packages: write` remains scoped to the `publish-main` job.

Validation command used: a PowerShell temporary-directory harness initialized a temporary Go module, pinned `go.yaml.in/yaml/v2@v2.4.2`, unmarshaled `.github/workflows/docker-publish.yml`, and removed its temporary directory. Exact output:

```text
go: creating new go.mod: module workflowcheck
go: added go.yaml.in/yaml/v2 v2.4.2
valid YAML: C:\Users\wende\Projects\tusk\tusk-engine\.worktrees\automated-engine-releases\.github\workflows\docker-publish.yml
```

`git diff --check` passed. Manual inspection confirmed `workflow_call` is the only trigger and `packages: write` is present only on the publication job. The Task 5 caller must still enforce aggregator verification for the exact SHA before invoking this reusable workflow.
