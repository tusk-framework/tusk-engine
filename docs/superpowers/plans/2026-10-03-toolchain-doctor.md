# Toolchain Doctor and Project Resolution

## Goal

Give Tusk Engine a deterministic, inspectable answer to “which PHP,
Composer, and RoadRunner will this project use?” before provisioning is
implemented.

## Scope

- Read an optional `.tusk/toolchain.json` manifest.
- Prefer existing project-local binaries declared by that manifest.
- Fall back to system `PATH` lookup for PHP, Composer, and RoadRunner.
- Capture executable path, source, detected version, requested version, and
  status in a structured report.
- Expose the report through `tusk doctor` in human-readable and JSON forms.
- Reuse the same report from `tusk setup`.

## Explicit non-goals

- No network downloads.
- No executable replacement or PATH mutation.
- No version-manager integration.
- No attempt to interpret arbitrary Composer version constraints yet; the
  first slice treats a manifest version as a visible pin and checks whether
  its normalized text occurs in the detected version string.

## Verification

- Unit tests cover project-over-system resolution, missing tools, version
  mismatch, malformed manifests, and JSON output.
- Run `gofmt`, `go test ./...`, `go vet ./...`, `go build ./...`, and
  `git diff --check`.
