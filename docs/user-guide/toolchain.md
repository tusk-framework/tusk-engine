# Toolchains and setup

The Engine supports explicit toolchain pins in `.tusk/toolchain.json`,
diagnostics through `tusk doctor` (or `tusk doctor --json`), and explicit
provisioning through `tusk setup --toolchain`. A relative executable path in
the project manifest takes precedence over a matching executable on `PATH`.
The Engine does not modify the machine-wide `PATH`.

```sh
tusk doctor
tusk setup --toolchain
tusk setup --toolchain --offline
tusk toolchain list
tusk toolchain pin php@8.3.23
```

Provisioning is explicit. With pins in `.tusk/toolchain.json`, online setup
fetches the toolchain catalog asset for the exact Engine release, verifies its
release-embedded SHA-256 and signature, caches it atomically, then provisions
the requested tools. It never falls back to a stale catalog after a fetch or
verification failure. `--offline` never accesses the network and requires a
cached catalog whose bytes match the same digest and signature, plus verified
artifacts in the local cache. If no project-local tools are pinned, setup
finishes without catalog access. `tusk start`, ordinary diagnostics, and
`tusk docs` do not provision tools implicitly.

## Profiles and current support

The project-local profile uses pinned, verified artifacts where the official
catalog provides them. At present, managed PHP bundle coverage is limited;
consult `tusk doctor` for the exact target and available artifact. Linux and
macOS may use an explicitly configured system or Docker profile. Do not infer
that PHP is bundled on a platform just because the Engine binary runs there.

The system profile uses tools already installed by the developer. Docker
profiles keep runtime dependencies in the container. Both are explicit
operating modes; setup does not install operating-system packages or mutate
global configuration.

## Pins and resolution

Keep `.tusk/toolchain.json` in source control when the project requires
reproducible versions. Resolution prefers a valid project-local executable,
then uses the configured profile's permitted system executable. Use
`tusk toolchain list` and `tusk doctor` to inspect what was selected. Never
commit downloaded runtime binaries or cache contents. `catalog_version` is
inventory metadata and need not match the Engine release version; the exact
signed catalog is instead bound to each release binary by its SHA-256 digest.

Project-local PHP bundles across Windows, Linux, and macOS, as well as a
curated extension profile for every advertised target, are planned work and
are not yet available. Follow `tusk docs security` for the verification model.
