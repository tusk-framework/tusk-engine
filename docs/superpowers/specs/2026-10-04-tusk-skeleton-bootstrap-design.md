# Tusk Skeleton and Application Bootstrap

## Status

Design approved in conversation on 2026-10-04. This document defines the
architecture for the official Tusk project skeleton and the bootstrap contract
between `tusk-engine` and `tusk-framework`.

## Context

Tusk now uses a Go Engine to supervise RoadRunner while PHP owns application
composition and request handling. The current repository still exposes older
runtime concepts: a root `worker.php`, direct file execution, and a native
NDJSON adapter. Those concepts make a RoadRunner application look different
from familiar PHP and modern application frameworks and can cause a worker
written for the native protocol to be started by RoadRunner.

The desired developer experience is a conventional application skeleton with a
single application bootstrap, familiar configuration locations, and a runtime
worker that is generated and owned by the platform. RoadRunner, Goridge, worker
pools, and request-scope mechanics must remain implementation details.

## Decision

The official Tusk skeleton will use `bootstrap/app.php` as the application
composition root. The Engine will generate an internal RoadRunner worker at
`.tusk/runtime/worker.php`. The generated worker loads Composer, requires the
application bootstrap, and enters the RoadRunner runtime through the Tusk
Application API.

The user-facing runtime command is:

```text
tusk start
  -> Tusk Engine
  -> RoadRunner
  -> .tusk/runtime/worker.php
  -> bootstrap/app.php
  -> Tusk Application
```

RoadRunner is the only supported default server path. The native NDJSON
adapter remains migration-era or test-only code and is not selected by the
generated worker or by `tusk start`.

## Goals

- Make a new Tusk project understandable to PHP developers familiar with
  Laravel, Symfony, Spring Boot, .NET, or Node.js.
- Expose one public application bootstrap contract:
  `bootstrap/app.php` returns a configured Tusk application.
- Keep runtime infrastructure out of normal application code.
- Make `tusk start` work with a generated worker and no hand-written runtime
  loop.
- Keep `tusk.json` focused on Engine/platform settings and `config/*.php`
  focused on application configuration.
- Preserve a conventional `public/index.php` for compatibility and tooling
  that expects a traditional PHP HTTP entrypoint.
- Provide actionable errors when the bootstrap is missing, invalid, or cannot
  be loaded.
- Preserve persistent workers and reset request-scoped state after every
  request.

## Non-goals

- Reimplementing RoadRunner's HTTP server, pool, Goridge protocol, recycling,
  or process supervision in PHP.
- Making `config.php` a special or mandatory filename.
- Supporting silent fallback from RoadRunner to the native runtime.
- Adding a second public runtime abstraction in the first skeleton rollout.
- Removing all legacy native code in the same change; it will remain isolated
  until migration coverage is complete.

## Project layout

The generated project layout is:

```text
my-app/
├─ app/
│  ├─ Http/
│  ├─ Providers/
│  └─ ...
├─ bootstrap/
│  └─ app.php
├─ config/
│  ├─ app.php
│  ├─ database.php
│  └─ ...
├─ public/
│  └─ index.php
├─ routes/
│  ├─ web.php
│  └─ api.php
├─ storage/
├─ composer.json
├─ tusk.json
└─ .tusk/
   └─ runtime/
      └─ worker.php
```

The `.tusk` directory is generated state and must be ignored by project Git
templates except for files explicitly documented as user-owned. The generated
worker must be recreated when its template or the application root changes.

## Public PHP bootstrap contract

The skeleton bootstrap uses a builder-style API:

```php
<?php

use Tusk\Foundation\Application;

return Application::configure(dirname(__DIR__))
    ->withRouting(
        web: __DIR__ . '/../routes/web.php',
        api: __DIR__ . '/../routes/api.php',
    )
    ->withProviders([
        App\Providers\AppServiceProvider::class,
    ])
    ->create();
```

The contract is deliberately small:

- `Application::configure(string $basePath)` returns a builder.
- `withRouting(...)`, `withProviders(...)`, and later builder methods are
  configuration steps and do not start a server.
- `create()` returns the application object.
- The returned application owns the container, configuration, providers,
  router, and kernel.
- The application exposes a framework-level request handler for HTTP adapters
  and a worker entrypoint used only by the generated runtime worker.

The bootstrap must be safe to load once per PHP worker. Per-request mutable
state belongs to request scope and must not be stored in singleton services.

## Generated worker contract

The Engine-generated `.tusk/runtime/worker.php` has one responsibility: bridge
RoadRunner to the application. Its conceptual form is:

```php
<?php

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$application = require dirname(__DIR__, 2) . '/bootstrap/app.php';

if (!$application instanceof \Tusk\Foundation\Application) {
    throw new \RuntimeException(
        'bootstrap/app.php must return a Tusk application instance.'
    );
}

$application->runWorker();
```

The generated worker uses the RoadRunner adapter internally. It must not
instantiate `NativeLoopAdapter`, read NDJSON from STDIN, or expose a second
application bootstrap convention.

The Engine passes the generated worker as the RoadRunner server command and
keeps the project root as the child working directory. It must not use the
Engine repository's legacy `worker.php` as an implicit application worker.

## Configuration boundaries

Configuration has four explicit ownership levels:

| Location | Owner | Responsibility |
| --- | --- | --- |
| `tusk.json` | Tusk Engine | Port, worker count, runtime limits, RoadRunner control, toolchain, and platform settings |
| `config/*.php` | Tusk application | Database, queues, cache, application services, and environment-backed values |
| `bootstrap/app.php` | Tusk application | Application composition, providers, routes, middleware, and kernel construction |
| `.tusk/runtime/*` | Tusk Engine | Generated RoadRunner configuration, worker, and ephemeral lifecycle state |

`composer.json` remains the dependency and script authority. Tusk may read its
metadata, but it does not replace Composer's dependency solver or lockfile.

## HTTP compatibility entrypoint

`public/index.php` remains a conventional entrypoint for compatibility and
development tools. Its implementation loads `bootstrap/app.php`, creates a
request from PHP globals, and delegates to the application HTTP handler. It
does not own the RoadRunner loop and is not used by `tusk start`.

This permits traditional tools to understand the project without making the
traditional per-request model the production runtime.

## Lifecycle and isolation

Application creation occurs once for each PHP worker process. Request handling
then follows this sequence:

```text
RoadRunner waitRequest
  -> application request handler
  -> response
  -> request-scope reset
  -> next request
```

The adapter must:

- convert RoadRunner requests to the framework request contract;
- convert framework responses back to RoadRunner responses;
- report uncaught request exceptions using the runtime's error policy;
- reset request-scoped services in a `finally` block;
- stop cleanly when RoadRunner closes the worker channel;
- run application shutdown hooks exactly once per worker.

The Go Engine remains responsible for process-level graceful shutdown. PHP
does not create or supervise a second worker pool.

## CLI responsibilities

The Go `tusk` binary is the canonical runtime CLI:

```text
tusk init
tusk dev
tusk start
tusk build
tusk test
tusk doctor
```

The PHP package CLI remains useful for framework generation and build commands,
but it must not start a competing native runtime. Runtime commands exposed by
the PHP CLI either delegate to the Go Engine or are removed from the generated
application command set.

## Failure behavior

Startup must fail before RoadRunner accepts traffic when:

- `bootstrap/app.php` does not exist;
- Composer autoloading cannot be loaded;
- the bootstrap returns a value that is not a Tusk application;
- provider construction or configuration fails;
- a route or middleware definition is invalid;
- the generated worker cannot be written safely.

Errors must include the project root, expected file, and a corrective action
such as `tusk init`, `composer install`, or editing `bootstrap/app.php`. No
failure may silently switch to the native adapter or an echo worker.

## Migration

Existing projects with a root `worker.php` are supported through an explicit
migration path, not implicit runtime selection:

1. `tusk doctor` identifies legacy native workers.
2. `tusk migrate` creates `bootstrap/app.php`, the standard directories, and
   the generated-worker metadata without overwriting user files.
3. The user confirms or adapts application composition.
4. `tusk start` uses `.tusk/runtime/worker.php`.

The migration command must refuse to overwrite existing `bootstrap/app.php`,
`config/`, or `routes/` files. Legacy worker support may remain available for a
bounded compatibility window, but it must be explicit and must not be the
default generated path.

## Testing strategy

### Engine tests

- generated worker path is inside the project `.tusk` directory;
- generated content is deterministic and does not contain secrets;
- Engine passes the generated worker to RoadRunner;
- Engine never falls back to the repository legacy worker;
- missing bootstrap produces an actionable startup error;
- generated files are cleaned only when owned by the Engine;
- `tusk.json` runtime settings remain separate from application config.

### Framework tests

- `Application::configure(...)->create()` returns a valid application;
- bootstrap files can be loaded repeatedly in isolated worker processes;
- `runWorker()` selects RoadRunner and never NativeLoopAdapter;
- request-scoped services are reset after success and failure;
- shutdown hooks run exactly once;
- `public/index.php` delegates to the same application handler.

### Skeleton smoke test

The generated project must pass this sequence in CI:

```text
tusk init
composer install
tusk build
tusk start
HTTP request to the configured port
graceful shutdown
```

The smoke test must use the same RoadRunner path documented for production.

## Rollout order

1. Add the PHP `Application` builder and bootstrap contract in
   `tusk-framework`.
2. Add the generated worker template and safe file generation in
   `tusk-engine`.
3. Change the RoadRunner projection to invoke the generated worker.
4. Update the project generator and Docker packaging.
5. Add migration diagnostics for legacy workers.
6. Update documentation and the skeleton smoke test.
7. Deprecate native runtime entrypoints after migration coverage is released.

The first implementation must keep the public contract small. Additional
features such as environment profiles, automatic configuration discovery, and
provider grouping should build on this bootstrap rather than introduce new
entrypoint files.
