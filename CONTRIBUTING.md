# Contributing to Tusk Engine

Thank you for helping improve Tusk Engine. The Engine is the Go control plane
that manages the application runtime, RoadRunner, PHP workers, toolchain
provisioning, health checks, observability, and lifecycle operations.

Please read the [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Ways to contribute

You can help by:

- reporting reproducible bugs;
- proposing focused improvements through issues;
- improving documentation and examples;
- adding tests or improving diagnostics;
- implementing features that fit the Engine's runtime boundaries; or
- reviewing pull requests and release changes.

For security vulnerabilities, exposed credentials, or trust-chain concerns,
follow [`SECURITY.md`](SECURITY.md) and do not open a public issue.

## Before you start

1. Search existing issues and pull requests before opening a new one.
2. For a substantial change, open an issue first and describe the problem,
   proposed behavior, compatibility impact, and acceptance criteria.
3. Keep changes focused. Separate unrelated refactors, formatting changes,
   and feature work into different pull requests.
4. Never commit private keys, tokens, credentials, generated release assets,
   or local caches.

## Development environment

The Engine currently targets Go 1.23. A PHP 8.3 installation, Composer, and
RoadRunner are useful for the generated-skeleton smoke test.

Clone the repository and run the normal verification commands from the
repository root:

```bash
go test ./...
go vet ./...
go build ./...
git diff --check
```

When changing the official toolchain catalog, also run the payload validation:

```bash
go run ./cmd/tusk-catalog validate --payload release/toolchain-catalog.payload.json
```

The download verification mode contacts every unique artifact URL and checks
the committed SHA-256 digest. Use it when changing artifact metadata or before
a release:

```bash
go run ./cmd/tusk-catalog validate \
  --download \
  --payload release/toolchain-catalog.payload.json
```

The full CI suite also exercises the Docker image and the generated PHP
skeleton under RoadRunner. If a local prerequisite is unavailable, explain it
in the pull request instead of weakening the test or silently skipping it.

## Project boundaries

Keep responsibilities explicit:

- Tusk Engine owns control-plane concerns such as configuration, lifecycle,
  health checks, metrics, logging, runtime selection, and secure provisioning.
- RoadRunner owns PHP worker execution, worker pools, request handling, and
  worker recycling.
- PHP applications and the Framework own application behavior and framework
  conventions.

Changes that duplicate RoadRunner responsibilities, weaken the signed catalog,
or expose a new network surface require extra design discussion and security
review.

## Branches and commits

Use a descriptive branch name without a `codex/` prefix. Examples:

```text
feat/catalog-release-automation
fix/worker-shutdown-timeout
docs/contributing-guide
```

Use [Conventional Commits] for commit messages. The type controls automated
release versioning:

- `fix:` produces a patch release;
- `feat:` produces a minor release;
- `feat!:` or a `BREAKING CHANGE:` footer produces a major release;
- `docs:`, `test:`, `chore:`, and similar non-release commits do not create a
  release by themselves.

Use an imperative, specific subject, for example:

```text
feat: verify RoadRunner artifact digests before install
```

## Pull requests

A pull request should explain:

- what changed and why;
- the user-visible or operational impact;
- the packages, commands, workflows, or contracts affected;
- how the change was verified; and
- risks, compatibility concerns, migrations, and deliberate follow-ups.

Before requesting review:

- rebase or merge the current `main` as appropriate for the branch;
- run the relevant local verification commands;
- add or update tests for changed behavior;
- update documentation and examples when configuration or behavior changes;
- review the diff for secrets, unrelated changes, and accidental generated
  files; and
- confirm that the PR title and commits follow Conventional Commits.

Keep review discussions technical, specific, and respectful. A review comment
should identify the behavior or risk, explain why it matters, and suggest a
clear path forward when possible.

## Tests and implementation expectations

New behavior should have a focused regression test. Prefer deterministic tests
with real domain behavior over tests that only assert mock interactions.
Changes involving timeouts, retries, worker lifecycle, signatures, artifact
selection, or filesystem boundaries should include failure-path coverage.

Run formatting and static checks before submitting:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
```

Do not use formatting-only changes to hide a behavioral change. If a legacy
area has existing style violations, keep the scope explicit and document it.

## Release and catalog changes

Release automation is driven by Conventional Commits and publishes only after
the required main-branch checks pass. Do not create or move release tags by
hand unless a maintainer has asked you to perform a release operation.

Catalog changes must include:

- an approved HTTPS source;
- a verified SHA-256 digest;
- a correct platform target and entrypoint;
- an update to the release documentation when the matrix changes; and
- validation through `tusk-catalog validate --download`.

Never commit signing keys. The production signing secret and public trust
anchors are configured through GitHub Actions secrets and repository variables.

## Questions

If you are unsure whether a change belongs in the Engine, open an issue with a
short design proposal before implementing it. Maintainers can help identify
the correct boundary between Tusk Engine, RoadRunner, the Framework, and the
PHP application.

[Conventional Commits]: https://www.conventionalcommits.org/en/v1.0.0/
