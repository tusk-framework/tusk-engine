# Toolchain and artifact security

The Engine's official toolchain catalog is signed. Provisioning verifies the
catalog trust anchor, artifact metadata, download origin, digest, and safe
extraction before making an artifact available. Keep the Engine current so
it has the trust anchors intended for the catalog it consumes.

Use `tusk setup --toolchain` only when you explicitly intend to acquire
artifacts. `tusk setup --toolchain --offline` uses verified local cache
contents and fails if a required artifact is absent. Never bypass a signature
or checksum failure, and do not copy an unverified executable into a project
toolchain directory.

The Engine does not edit system `PATH` or machine-wide PHP configuration.
Project toolchain manifests may be committed; downloaded tools and credentials
must not be. Do not put secrets in command-line arguments or publish them in
diagnostic output.

## Planned PHP bundle guarantees

Signed PHP runtime metadata will include the target, PHP and Zend module API,
extension inventory, host prerequisites, source provenance, and license
information. Release targets must run on clean native runners before they are
advertised. These PHP bundle and extension-profile guarantees are under
implementation planning; current PHP artifact coverage is narrower. Consult
`tusk doctor` and the release catalog rather than assuming support.

Private catalog signing keys belong only in the protected release environment.
The corresponding public trust anchors can be distributed with the Engine.
