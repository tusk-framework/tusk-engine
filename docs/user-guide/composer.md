# Composer workflows

Composer owns PHP dependency resolution, plugins, autoload generation,
`vendor/`, and `composer.lock`. The Engine provides convenience commands but
does not replace Composer's solver or rewrite dependency constraints.

```sh
tusk install
tusk add vendor/package
tusk remove vendor/package
tusk update
```

You can use Composer directly as well:

```sh
composer install
composer validate --strict
composer check-platform-reqs
```

Commit `composer.lock` for applications so installs use the resolved versions.
For libraries, follow Composer's package guidance. Declare PHP and extension
requirements in the package's `require` section; use `require-dev` for
development-only tools. Do not put dependency declarations in `tusk.json`.

## PHP selection

Composer is normally distributed as a PHAR and therefore runs under PHP. The
Engine's managed Composer command resolves the selected PHP and Composer
through its toolchain configuration. For direct `composer` shell commands,
your shell's PATH determines which installations are used. Verify that both
paths match the project before comparing results:

```sh
where php       # Windows
where composer  # Windows
which php       # Linux/macOS
which composer  # Linux/macOS
```

Invoking Composer through the selected managed PHP and digest-verified PHAR
consistently for all commands is planned follow-up work. Until then, do not
assume a globally installed Composer uses the same PHP as RoadRunner workers.
