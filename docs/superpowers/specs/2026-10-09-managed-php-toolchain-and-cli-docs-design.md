# Managed PHP Toolchain and CLI Documentation Design

## Status

Proposed architecture based on the approved product direction. This document
defines scope and boundaries; implementation plans and backlog issues follow
only after review.

## Goal

Let a Tusk project use a pinned, verified PHP runtime, required PHP extensions,
Composer, and RoadRunner without requiring the developer to install PHP or
Composer globally, while keeping RoadRunner as the request runtime and
Composer as the PHP dependency and lockfile authority. Make the complete,
version-matched user documentation available offline through the Tusk CLI.

## Current state

The Engine already has a signed-catalog/provisioner foundation, project-local
toolchain manifests, content-addressed caching, explicit setup, and
process-local command environments. The catalog currently provisions PHP for
Windows x64; Composer and RoadRunner have broader platform coverage. Linux and
macOS PHP remain system/Docker responsibilities. The CLI has concise `help`
output, while detailed guides live only in the repository. PHP extension
requirements are not yet a first-class part of toolchain selection and
diagnostics.

The Engine's runtime model remains RoadRunner supervising PHP CLI workers.
Consequently, PHP must exist in the project environment or its container even
when it is not installed on the host. Composer is a PHP application (normally
distributed as a PHAR), so it must run under the same selected PHP runtime as
the project.

## Product principles

- Prefer project-local, pinned, verified toolchains; never mutate the global
  `PATH` or replace system-managed tools.
- Keep acquisition explicit. `tusk start`, `tusk dev`, ordinary diagnostics,
  and package commands do not perform an implicit network installation.
- Make generated projects easy to start with a single documented setup flow.
- Keep one source of truth for dependency requirements and lockfiles.
- Fail with actionable diagnostics when a platform, PHP extension, or native
  library combination is unsupported.
- Treat downloaded runtimes, extensions, and Composer as executable supply
  chain inputs: authenticate metadata, pin digests, retain provenance, and
  publish platform/license information.
- Keep the system and Docker profiles supported as explicit alternatives;
  project-local provisioning is the recommended portable developer profile.

## Recommended architecture

### PHP runtime bundles

Extend the official signed catalog with relocatable PHP runtime bundles for the
supported Engine targets: Windows x64, Linux x64/arm64, and macOS x64/arm64,
subject to release validation. A bundle is more than a `php` executable: it
contains its matching extension binaries, default configuration, runtime
libraries that can safely be redistributed, and metadata describing the PHP
version, PHP/Zend module API, target OS/architecture/libc, extension inventory,
and remaining host prerequisites.

Initially publish curated, reproducibly built extension profiles rather than
arbitrary extension downloads. The standard profile must satisfy the
requirements declared by the Framework packages and the generated skeleton.
Integration-specific extensions belong in additional profiles only when Tusk
documents and supports the corresponding integration. A profile is selected
only if its declared inventory satisfies the project's requirements and its
ABI/platform exactly matches the target. Do not silently combine binaries
from different PHP builds.

An extension not present in a supported profile produces a diagnostic naming
the extension and platform. Installing arbitrary PECL source, compiling
extensions on first run, or downloading an unreviewed DLL/shared object is
outside the initial design. Optional extensions may later be distributed as
separately signed, ABI- and platform-specific artifacts if a safe dependency
and compatibility model is demonstrated.

The catalog and release process must account for native OS prerequisites. A
project-local PHP bundle removes the need for a separately installed PHP
distribution; it must not claim to eliminate operating-system libraries or
vendor runtimes unless the bundle actually includes them and the license
permits redistribution. `doctor` reports any remaining prerequisite and links
to the relevant guide.

### Extension requirement authority and validation

`composer.json` remains the authority for application platform requirements.
The Framework's own package metadata must declare every extension required by
its runtime APIs with Composer `ext-*` constraints. Applications declare
additional extensions in their normal Composer `require`/`require-dev` sections.
The Engine must not introduce a second required-extension list that can drift
from Composer metadata.

The Engine uses Composer's real platform checks against the selected PHP
runtime after dependencies are installed. Provide clear commands/diagnostics
for at least:

- runtime-only validation, excluding development dependencies;
- full development/test validation;
- a pre-start check that reports missing extensions before launching
  RoadRunner workers.

Where the lockfile or installed vendor tree is unavailable, diagnostics may
combine the Framework's declared baseline with direct application
`ext-*` requirements, but must label this as a partial check. Composer's
`config.platform` is not evidence that an extension is physically loaded.
The definitive deployment check must inspect the real selected PHP runtime.

The PHP bundle supplies a project-local `php.ini`/scan directory and
`extension_dir`; the Engine sets `PHPRC` and any scan-directory environment
variables only for child processes. It must not alter machine-wide PHP
configuration. Extension startup warnings and failed module loads must be
reported as setup/doctor failures, not hidden by Composer platform emulation.

### Composer and the Tusk CLI

Provision Composer as a pinned, digest-verified PHAR through the existing
toolchain mechanism. The Engine invokes it explicitly through the resolved
project-local PHP executable and passes a process-local environment. Existing
commands (`tusk install`, `add`, `remove`, `update`, framework commands, and
Composer scripts) remain convenient Tusk entry points, but Composer continues
to own dependency solving, plugin execution semantics, `vendor/`, and
`composer.lock`.

The Engine may add focused platform-check and environment-management commands
when they improve the workflow. It must not fork Composer's resolver, silently
rewrite dependency constraints, or embed a second independently updated PHP
runtime inside the Go CLI. A missing managed Composer binary should be
repairable with the explicit setup command and should have a documented system
fallback where the selected profile allows one.

### Project setup and runtime resolution

Generated projects pin compatible PHP, Composer, and RoadRunner versions in
the existing `.tusk/toolchain.json` contract. Their setup flow provisions all
required artifacts from the official signed catalog, configures the managed
PHP extension environment, validates Composer platform requirements, and
prints a clear summary of installed versions and capabilities.

Resolution remains deterministic: explicit project path, matching
project-local artifact, then profile-permitted system executable. In CI,
project-local versions are pinned and verified. Docker profile behavior
continues to use container tools and never installs host toolchains. Offline
setup succeeds only from the verified cache and gives a precise missing-cache
diagnostic otherwise.

Starting the application uses RoadRunner and the same resolved PHP selected
for setup and Composer commands. The Engine does not take over worker pooling,
PHP lifecycle internals, or request serving from RoadRunner.

### Offline documentation through the CLI

Add a user-guide documentation tree (separate from design specs and
implementation plans) written in Markdown and included in the Engine build
using Go's embedded-file mechanism. The Markdown files in the repository are
the single source of truth for both offline CLI reading and any later website
publishing. If Go's package-local embed rules require generated assets, use a
deterministic generation/check step and test that the embedded output matches
the Markdown sources; do not maintain a second manually edited copy.

The initial CLI contract is:

```text
tusk docs                 Show the documentation index and available topics
tusk docs <topic>         Read a guide from the installed Engine version
tusk help                 Show concise command help and point to tusk docs
```

Documentation is available without network access, a browser, PHP, or
Composer. Topics should include quick start and project layout; toolchain
profiles and pinning; PHP bundle/platform prerequisites; extension requirements
and troubleshooting; Composer usage and lockfile ownership; RoadRunner runtime
and worker lifecycle; setup/doctor/offline-cache workflows; security and
artifact verification; CI and Docker; upgrade and migration guidance. CLI docs
must identify the Engine/documentation version so users can distinguish local
guidance from newer online material. Online opening/search integration is
optional future work, not a dependency of the offline documentation feature.

## Alternatives considered

1. **Keep requiring system PHP and Composer.** Lowest maintenance, but does not
   meet the project's goal of a predictable first-run developer experience.
2. **Use curated project-local PHP bundles and managed Composer (recommended).**
   Extends the existing signed toolchain architecture, supports offline use,
   and preserves RoadRunner/Composer responsibilities. The cost is maintaining
   trusted platform-specific bundles and testing their ABI/native dependencies.
3. **Compile PHP and arbitrary extensions on the user's machine, or embed PHP
   in the Go Engine / switch to another server runtime.** This increases build
   and operational complexity, weakens reproducibility, or changes the
   established RoadRunner architecture. Do not choose this as the initial
   implementation.

## Security, licensing, and operations

- Reuse the existing official catalog signature, immutable trust anchors,
  HTTPS allow-list, digest verification, bounded downloads, safe extraction,
  and atomic installation.
- Review redistribution terms for PHP, each compiled extension, and required
  native libraries; publish license/source metadata and a component inventory.
- Keep private signing material out of the repository and release artifacts.
- Never load user-selected extension binaries from untrusted paths as part of
  official provisioning.
- Keep project-local PHP configuration isolated and avoid leaking secrets in
  setup summaries or diagnostics.
- Test with a clean host that has no PHP/Composer on `PATH`, as well as with
  conflicting system versions, offline cache, malformed bundles, missing
  extensions, and unsupported targets.

## Delivery boundaries and backlog decomposition

This is a coordinated product capability, not one implementation PR. The
backlog should separate independently reviewable work while preserving these
dependencies:

1. Define PHP bundle build, extension-profile metadata, OS prerequisites,
   redistribution review, provenance, and catalog entries for supported
   targets.
2. Extend manifest/setup/doctor to select bundles, configure PHP locally, and
   diagnose the exact PHP/extension/native-library mismatch.
3. Integrate Composer validation and all package/script commands with the
   managed PHP consistently, preserving Composer as authority.
4. Add embedded offline `tusk docs`, complete user guides, and CLI help links.
5. Add clean-host cross-platform integration coverage and update generated
   skeletons, Docker guidance, release operations, and migration docs.

The first deliverable should prove the existing Windows x64 PHP path
end-to-end with curated extension metadata and then add Linux/macOS targets in
independently verifiable slices. Platform-specific PHP packaging should be
tracked separately from Engine command integration if their owners/release
cadence differ; a platform must not be advertised until its artifact and
clean-host smoke test pass.

## Non-goals

- Replacing RoadRunner or changing the PHP worker protocol.
- Reimplementing Composer's package resolver or lockfile format.
- Mutating global `PATH`, machine-wide `php.ini`, or installing OS packages
  without an explicit user action.
- Downloading or compiling arbitrary extensions during normal startup.
- Network access from `tusk start`, ordinary `doctor`, or `tusk docs`.
- Promising one universal PHP executable that is independent of all host OS
  libraries and vendor runtimes.

## Acceptance criteria

- A supported project can use `tusk setup --toolchain` on every advertised
  target without a globally installed PHP or Composer.
- The selected runtime, Composer invocation, and RoadRunner workers use the
  same pinned PHP version and project-local extension configuration.
- Composer's real platform check detects missing or incompatible PHP/ext
  requirements before deployment/startup, with an actionable Engine message.
- Unsupported extensions/platform combinations fail closed and name the
  missing capability; no arbitrary extension binary is installed.
- Setup verifies official trust, artifact digests, metadata, target and
  extension inventory before publishing a usable runtime.
- Existing system and Docker profiles remain compatible and do not acquire
  unintended host-side effects.
- `tusk docs` and `tusk docs <topic>` work offline from the built Engine
  binary; user-facing Markdown is the single source of truth and matches the
  binary's version.
- Clean-host integration tests cover each advertised OS/architecture and prove
  that no global PHP or Composer is required.
