# Security Policy

## Supported versions

Security fixes are applied to the current `main` branch and the latest
published Engine release. Older releases may not receive fixes; upgrade before
reporting a vulnerability when possible.

## Reporting a vulnerability

Do not disclose vulnerabilities, exposed credentials, signing-key concerns, or
trust-anchor issues in a public issue or pull request. Use GitHub's private
security reporting flow for `tusk-framework/tusk-engine`, or contact the
project maintainers privately through the Tusk organization.

Please include the affected version or commit, the runtime and platform, a
minimal reproduction, the security impact, and any suggested mitigation. Do
not include real private keys or production credentials in the report.

Reports are reviewed privately and contributors will be kept informed when it
is safe to share progress. Public disclosure should be coordinated with the
maintainers after a fix or mitigation is available.

## Release and catalog security

Engine catalog releases use digest-pinned HTTPS artifacts and signed catalog
metadata. Private signing material is stored only in protected GitHub Actions
secrets. Public trust anchors are repository configuration and may be rotated
without exposing private keys.

Never commit signing keys, tokens, credentials, generated signed catalogs, or
local secret files. If one is exposed, report it privately immediately so the
credential can be revoked and the trust chain rotated.
