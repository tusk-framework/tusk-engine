# PHP extensions and platform requirements

PHP extensions are loaded by the selected PHP executable and must match that
PHP build's ABI and operating system. Check the actual runtime before
diagnosing an application:

```sh
php -v
php -m
tusk doctor
```

Composer package metadata should declare required extensions using `ext-*`
constraints in `composer.json`. Composer's `config.platform` can emulate a
platform during dependency solving; it does not prove that an extension is
loaded by the real PHP process.

## Current Engine behavior

The Engine diagnoses tool availability and starts RoadRunner with the
configured PHP executable. It does not yet publish a signed extension
inventory for managed PHP or provide `tusk check-platform-reqs`. Until that
capability lands, run Composer's platform check explicitly:

```sh
composer check-platform-reqs
composer check-platform-reqs --no-dev
```

The first checks development and runtime dependencies; `--no-dev` checks only
production requirements. Run the command with the same PHP executable that
will serve the application. A missing extension should be installed through a
trusted PHP distribution or container image appropriate to the project; the
Engine does not download arbitrary PECL binaries.

Project-local `php.ini` isolation, authenticated extension profiles, and a
pre-start check against Composer's real platform requirements are planned.
They are not implied by the current setup command.
