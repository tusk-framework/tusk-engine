# Tusk Engine user guide

These guides describe the Engine contract at the version shown by `tusk docs`.
The Engine manages project setup and RoadRunner's lifecycle; RoadRunner serves
requests and owns PHP worker pooling. Composer remains the dependency and
lockfile authority.

## Quick start

1. Use a modern Tusk Framework project containing `bootstrap/app.php`.
2. Ensure PHP and RoadRunner are available. The Engine can provision supported
   project-local tools explicitly with `tusk setup --toolchain`; see
   [Toolchains](toolchain.md) for current platform coverage.
3. Install dependencies with `tusk install` or `composer install`.
4. Run `tusk doctor`, then `tusk start`.

`tusk init` creates Engine configuration; it does not generate a PHP
application. `tusk start` does not download or install tools implicitly.

## Topics

- [Toolchains, pins, setup, and cache](toolchain.md)
- [PHP extensions and platform requirements](php-extensions.md)
- [Composer and dependency workflows](composer.md)
- [Bootstrap, RoadRunner, and workers](runtime.md)
- [Catalog, artifact, and supply-chain security](security.md)
- [CI and Docker profiles](ci-docker.md)
- [Upgrading projects and the Engine](upgrading.md)

Read any guide without a network connection using `tusk docs <topic>`, for
example `tusk docs runtime`.
