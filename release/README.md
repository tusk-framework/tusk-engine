# Official toolchain catalog release operations

The official catalog is a signed release asset, not a project configuration
file. `release/toolchain-catalog.payload.json` is the reviewed input to the
release process. It must contain real, HTTPS-only, digest-pinned PHP, Composer,
and RoadRunner artifacts for the supported platforms. The payload is never
accepted directly by the Engine and must not be copied into a project as a
trusted catalog.

## First-release prerequisites

Before creating the first release, an operator must complete these steps
outside this repository:

1. Generate an Ed25519 signing key using an approved key-management process.
2. Store the base64 private key only in the GitHub Actions secret
   `TUSK_CATALOG_SIGNING_KEY_B64`. The workflow reads it from the environment;
   it never accepts a private-key file and never writes the private key to the
   checkout, artifacts, logs, or release assets.
3. Store a base64 JSON trust-anchor document in the Actions variable
   `TUSK_CATALOG_TRUST_ANCHORS_B64`. Its public-only shape is
   `{"keys":{"<key-id>":"<base64-ed25519-public-key>"}}`.
4. Store the selected key ID in the Actions variable `TUSK_CATALOG_KEY_ID`.
5. Review and commit `release/toolchain-catalog.payload.json` with real
   artifact metadata and digests.

Missing payload, key ID, signing secret, or public trust anchors stops the
workflow before GoReleaser publishes anything.

## Reproducible signing

The signing command uses only canonical JSON and standard-library Ed25519 and
SHA-256 operations:

```bash
TUSK_CATALOG_SIGNING_KEY_B64="$..."
TUSK_CATALOG_KEY_ID="$..."
TUSK_CATALOG_TRUST_ANCHORS_B64="$..."
export TUSK_CATALOG_SIGNING_KEY_B64 TUSK_CATALOG_KEY_ID TUSK_CATALOG_TRUST_ANCHORS_B64

go run ./cmd/tusk-catalog sign \
  --payload release/toolchain-catalog.payload.json \
  --catalog release/toolchain-catalog.json \
  --provenance release/toolchain-catalog.provenance.json
go run ./cmd/tusk-catalog verify --catalog release/toolchain-catalog.json
```

The signing key must match the public key under the selected key ID. Artifact
signatures cover artifact metadata with the signature field empty, and the
catalog signature covers the complete typed payload. Repeating the operation
with identical payload bytes and key material produces identical signed bytes;
the provenance file deterministically records the payload digest, signed
catalog digest, and key ID.

The shell values above are secret references/placeholders, not key material.
Never replace them with a committed key file, command-line private key, or
real secret in documentation, tests, or workflow YAML.

## Rotation and revocation

For a planned rotation, add the new public key to
`TUSK_CATALOG_TRUST_ANCHORS_B64` while retaining the old key during the overlap
window. Publish a catalog signed with the new key and release Engine binaries
that embed both public keys. After every catalog signed by the old key has
expired and the supported clients have moved to the new release, publish a
later Engine build whose anchor set removes the old key. The overlap avoids
making a still-valid catalog unverifiable during rollout.

For emergency revocation, remove the compromised key ID from the public trust
anchor variable and publish a new Engine release. Builds containing the
revised anchor set reject catalogs signed by that key, including otherwise
well-formed catalogs. Record the reason, effective time, replacement key ID,
and last accepted catalog version in the release incident record; do not put
private material in that record or in this repository.

Trust anchors are injected into release binaries through Go linker flags and
contain public keys only. A development build without injected anchors fails
closed, and an unsigned or unknown-key project-local catalog is never treated
as official.
