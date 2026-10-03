# Tusk Engine Toolchain Provisioning

## Status

Approved architectural direction; implementation starts after review of this
specification.

## Goal

Allow a Tusk project to acquire and select reproducible PHP, Composer, and
RoadRunner executables without requiring a global installation and without
silently replacing tools owned by the developer or operating system.

The Engine remains the lifecycle and toolchain control plane. Composer remains
the PHP dependency solver and lockfile authority. RoadRunner remains the
request/worker runtime.

## User-facing contract

Existing diagnostic commands remain read-only:

```text
tusk doctor
tusk doctor --json
tusk toolchain list
tusk toolchain pin php@8.3
```

Provisioning is explicit:

```text
tusk setup --toolchain
tusk setup --toolchain --offline
```

Provisioning must never happen as a side effect of `start`, `dev`, package
commands, or ordinary `doctor` execution.

The project manifest remains declarative:

```json
{
  "php": { "version": "8.3" },
  "composer": { "version": "2.8" },
  "roadrunner": { "version": "2025.1" }
}
```

An optional `path` continues to select an already managed or manually supplied
binary. Provisioning may add a validated project-local path after installation,
but must not discard an explicit path supplied by the user.

## Resolution and installation layout

Resolution order for each tool is:

1. an existing explicit `path` from `.tusk/toolchain.json`;
2. a validated project-local installation for the requested version;
3. a system executable resolved through `PATH`;
4. provisioning, only when the user explicitly invokes `setup --toolchain`.

The Engine does not mutate the global `PATH`. Project-local installations use:

```text
.tusk/
  cache/
    artifacts/<sha256>
  toolchain/
    php/<version>/<os>-<arch>/php[.exe]
    composer/<version>/<os>-<arch>/composer.phar
    roadrunner/<version>/<os>-<arch>/rr[.exe]
```

Downloads are staged in a temporary sibling path, verified, extracted with
archive traversal protection, and atomically renamed into the final directory.
An interrupted installation must not be reported as usable.

## Trusted artifact catalog

The Engine consumes a signed catalog rather than assembling download URLs from
user input. Each artifact entry contains:

- logical tool and semantic version;
- target operating system and architecture;
- HTTPS URL;
- SHA-256 digest;
- Ed25519 signature over the canonical artifact metadata;
- archive format and expected executable path;
- optional license/source metadata for diagnostics.

The verification key is an immutable trust anchor shipped with the Engine.
Catalog refresh is allowed only when the catalog signature validates. Artifact
downloads are accepted only when the URL uses HTTPS, the host is in the
catalog's allowed source set, the digest matches, and the artifact signature
validates.

The first implementation must not silently fall back to unsigned metadata.
When a catalog or signature is unavailable, provisioning fails with an
actionable diagnostic and leaves the existing installation untouched.

## Component boundaries

The implementation is split into focused interfaces under `internal/toolchain`:

- `Catalog`: decode and verify signed catalog metadata;
- `Resolver`: choose project, cache, system, or provisioned candidates;
- `Downloader`: fetch bytes with bounded size and cancellation;
- `Verifier`: validate digest and signature before extraction;
- `Installer`: safely extract and atomically publish one tool;
- `Provisioner`: orchestrate independent PHP, Composer, and RoadRunner plans;
- `Store`: read/write manifest, cache metadata, and installation records.

The default adapters use the standard library. Tests inject fake catalog,
download, verification, filesystem, and platform adapters; no test depends on
the public internet.

## Failure and safety rules

- Existing binaries are never overwritten in place.
- A failed tool does not remove or invalidate successfully resolved tools.
- A failed installation leaves no executable-looking partial directory.
- Archive entries must remain below the destination root after clean path
  resolution; absolute paths and `..` traversal are rejected.
- Downloads have an explicit maximum size and request timeout.
- Cache entries are content-addressed by SHA-256 and are safe to reuse only
  after digest verification.
- Offline mode fails clearly when a verified cache entry is unavailable.
- Error output identifies tool, version, platform, source, and remediation,
  while JSON output remains machine-readable.

## Incremental delivery

1. Implement catalog/artifact models, signed metadata verification, cache
   storage, and safe archive extraction with unit tests.
2. Implement provisioning orchestration with injected adapters and local
   integration fixtures; wire `setup --toolchain` behind explicit flags.
3. Add official catalogs and platform packaging for Windows, Linux, and macOS.
4. Integrate provisioned paths into RoadRunner startup and `doctor` output.
5. Add CI/offline coverage and document key rotation and catalog release.

## Out of scope

- Reimplementing Composer dependency resolution.
- Running downloads during normal application startup.
- Replacing system package managers.
- A proxy, service mesh, or gateway layer in the Engine.
- Supporting arbitrary unsigned third-party catalogs in the default flow.

## Acceptance criteria

- A valid signed artifact can be cached, verified, extracted, and selected by
  a project without modifying global environment state.
- Tampered, unsigned, oversized, unsupported, or path-traversal artifacts are
  rejected before publication.
- Offline provisioning succeeds only from verified cache entries.
- Existing project/system binaries continue to work when provisioning is not
  requested.
- Unit and integration tests run without internet access.
