# Application bootstrap and RoadRunner runtime

The supported application contract uses `bootstrap/app.php`, which returns a
configured `Tusk\Foundation\Application` and does not start a server. From
the application root:

```sh
tusk doctor
tusk start
```

`tusk dev` is an alias for `tusk start`. Startup validates the project
bootstrap, generates the private worker at `.tusk/runtime/worker.php`, and
starts RoadRunner. Keep generated `.tusk` runtime state out of version control.

| Responsibility | Owner |
| --- | --- |
| Project settings, readiness, process lifecycle, diagnostics | Tusk Engine |
| HTTP serving, Goridge, worker pool, recycling, request limits | RoadRunner |
| Request handling, routes, middleware, application services | Framework application |
| Dependencies, autoloading, lockfile | Composer |

The Engine does not implement a second worker pool or HTTP data plane. Each
long-lived PHP worker loads the application once; the Framework must reset
request-scoped state between requests.

## Project files

- `bootstrap/app.php` composes and returns the application.
- `config/*.php` contains application configuration; there is no special
  root-level `config.php` bootstrap convention.
- `routes/*.php` contains application routes.
- `public/index.php` is not the RoadRunner worker loop.
- `tusk.json` contains Engine/platform settings and scripts.
- `composer.json` and `composer.lock` own dependencies and Composer scripts.
- `.tusk/runtime/worker.php` is generated Engine state; do not edit it.

`tusk init` creates `tusk.json`, not a complete application skeleton. For
deployment, provide the same PHP runtime, extensions, Composer-installed
dependencies, RoadRunner binary, and application files that passed validation
in CI. Project-local cross-platform PHP bundles are still planned; see
`tusk docs toolchain`.
