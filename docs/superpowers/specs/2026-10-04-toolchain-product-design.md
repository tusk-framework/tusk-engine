# Reproducible PHP Toolchain Product Design

## Goal

Deliver the product-facing toolchain workflow for PHP, Composer, and
RoadRunner: a versioned manifest, actionable doctor output, explicit setup,
independent resolution/provisioning, execution profiles, and process-local
PATH isolation on Windows, Linux, and macOS.

## Boundaries

The Engine owns tool selection, verified artifact installation, and process
lifecycle. Composer remains an external dependency authority: `install`,
`require`, `remove`, `update`, and test scripts execute Composer/PHP without
rewriting dependency resolution or `composer.lock`. RoadRunner remains the
data-plane runtime.

The product consumes a signed catalog through the existing `CatalogVerifier`.
It does not add a catalog, trust key, signing workflow, release job, key
rotation policy, or release artifact to this change; those belong to issue
#11. Until an official trusted catalog is available, setup fails before any
network or filesystem mutation with an actionable diagnostic.

## Manifest contract

`.tusk/toolchain.json` remains backward-compatible with the existing top-level
`php`, `composer`, and `roadrunner` tool specs. It gains:

```json
{
  "profile": "project-local",
  "platform": { "os": "windows", "arch": "amd64" },
  "php": { "version": "8.3.0", "path": ".tusk/toolchain/php/.../php.exe" },
  "composer": { "version": "2.8.11", "path": ".tusk/toolchain/composer/.../composer.phar" },
  "roadrunner": { "version": "2025.1.0", "path": ".tusk/toolchain/roadrunner/.../rr.exe" }
}
```

Profile values are `system`, `project-local`, `docker`, and `ci`. Tool pins
remain independent: a PHP failure does not suppress a Composer or RoadRunner
diagnosis/provision attempt. Explicit paths are never overwritten. Provisioned
paths are written only after the installer publishes a verified artifact.

## Resolution and execution

Resolution is per tool and follows explicit manifest path, matching local
installation, then profile-allowed system lookup. System lookup uses the
current process environment read-only. Project-local paths are normalized
against the project root and must not escape it when they refer to managed
installations.

Commands receive a derived environment through `exec.Cmd.Env`. The derived
PATH can prepend managed tool directories and `vendor/bin`, with the platform
separator, while the parent process environment is untouched. PHP and
RoadRunner startup use the resolved absolute paths. Composer PHAR execution
uses the resolved PHP executable as its interpreter when required, preserving
Composer as the command authority.

Profiles express policy, not a second installer:

- `system`: accept system PATH tools and report their ownership/source.
- `project-local`: prefer and isolate `.tusk/toolchain` installations.
- `docker`: use the container's configured paths/PATH; never install host tools.
- `ci`: require declared versions and verified/local candidates; fail clearly
  when offline cache or the trusted catalog is unavailable.

## Diagnostics

Human and JSON reports include profile, target OS/architecture, source, path,
detected/requested versions, verification state, and a remediation message.
Missing, mismatched, unsupported-platform, offline-cache, unsigned-catalog,
and path-isolation failures name the affected tool and suggest the exact next
command. Windows executable suffixes and PATH separators are handled without
hard-coded Unix paths.

## Verification

TDD tests cover manifest round-tripping and legacy compatibility, profile
policy, independent resolution, verified setup handoff, process-local PATH
construction, Composer authority, Windows/Linux/macOS path behavior, and
actionable CLI diagnostics. Existing catalog/cache/installer security tests
remain unchanged and the release workflows are not modified.
