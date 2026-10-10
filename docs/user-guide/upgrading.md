# Upgrading Tusk projects

Before upgrading the Engine or Framework, commit or back up application
changes, `composer.json`, `composer.lock`, and `.tusk/toolchain.json`. Read the
release notes for both repositories and verify the target PHP and RoadRunner
versions on the intended platform.

After updating versions:

```sh
tusk doctor
composer install
composer check-platform-reqs
```

Then run the application tests and a RoadRunner smoke request. Review changes
to `bootstrap/app.php`, generated `.tusk/runtime/worker.php` behavior, and
Framework package extension requirements. Do not edit generated worker state
to work around a compatibility issue.

## Toolchain migration status

The project-local PHP bundle and curated extension-profile work is planned.
Until those bundles are released for your OS and architecture, retain a
documented system or Docker PHP setup and pin supported versions in CI. Do not
remove a working system installation based only on roadmap documentation.

Offline CLI guides are versioned with the Engine binary. `tusk docs` describes
the version that is installed; consult that output when comparing a local
binary with newer online documentation.
