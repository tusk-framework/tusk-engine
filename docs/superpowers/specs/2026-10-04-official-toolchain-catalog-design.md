# Official Signed Toolchain Catalog Design

## Goal

Release the official Tusk toolchain catalog with a verifiable trust anchor,
reproducible signing/provenance tooling, and documented key rotation and
revocation. The Engine must fail closed when the official anchor is absent and
must never treat an unsigned project-local catalog as trusted.

## Scope and constraints

- Keep the existing `internal/toolchain` provisioner behavior unchanged.
- Reuse the existing Ed25519 catalog envelope and artifact signatures.
- Ship public trust material only; no private signing key is committed or
  written to the workspace by the release workflow.
- The official catalog is an authenticated release asset. A checked-in or
  project-local payload is input to the signing process only and is never
  accepted directly by the Engine.
- Builds without release configuration have no usable official trust anchor
  and fail with an actionable error.

## Design

The build contains a base64-encoded trust-anchor document through Go linker
flags. The document maps key IDs to Ed25519 public keys. `OfficialCatalogVerifier`
decodes that document and returns the existing `CatalogVerifier` configured to
require a non-empty validity window (`issued_at` and `expires_at`). A release
build may contain more than one key during a rotation overlap; a later build
revokes a key by removing it from the embedded set. The private key remains a
GitHub Actions secret and is consumed only through the signing command's
environment.

The signed payload gains `catalog_version`, `issued_at`, and `expires_at`
metadata. The existing canonical JSON rules remain the signing contract:
artifact signatures cover artifact metadata with its signature field empty,
and the catalog signature covers the complete typed payload. Unknown JSON
fields, invalid dates, expired catalogs, non-HTTPS URLs, host mismatches,
unsupported artifacts, and invalid signatures are rejected.

`cmd/tusk-catalog` signs a canonical payload, verifies that the signing key's
public half is present under the selected key ID in the configured trust
anchors, writes the signed catalog atomically, and writes deterministic
provenance containing payload/catalog SHA-256 digests and the key ID. It does
not accept a private-key file path. The workflow supplies the payload path and
secret/variable values explicitly.

The release workflow validates the payload, signs it, verifies it with the
same anchor document, creates GitHub build provenance for the catalog and
catalog provenance file, and gives GoReleaser the trust-anchor value through
`ldflags`. GoReleaser publishes the signed catalog and deterministic
provenance as release assets. Missing payload, key, key ID, or trust-anchor
configuration stops the release before publication.

## Testing

- Unit tests cover deterministic signing, artifact signatures, trust-anchor
  decoding, unsigned local catalog rejection, unknown/revoked key rejection,
  validity-window enforcement, and deterministic provenance.
- Existing provisioning tests remain unchanged and continue to use injected
  test keys/catalogs.
- Workflow checks are local and offline: `go test ./...`, `go vet ./...`,
  `go build ./...`, and `git diff --check`.

## Release-dependent work

This change cannot create the operator-owned key or real PHP/Composer/
RoadRunner artifact inventory. The first official release still requires:

1. generating the Ed25519 key outside the repository;
2. storing the private key as `TUSK_CATALOG_SIGNING_KEY_B64` and the public
   key set as `TUSK_CATALOG_TRUST_ANCHORS_B64`/`TUSK_CATALOG_KEY_ID` Actions
   configuration; and
3. committing a reviewed `release/toolchain-catalog.payload.json` containing
   real, digest-pinned HTTPS artifacts and supported platform entries.

