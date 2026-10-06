# Project runtime and ownership

Run `tusk start` from a modern Tusk project root after installing Composer
dependencies and making PHP and RoadRunner available. The project must contain
`bootstrap/app.php`, which returns a configured `Tusk\Foundation\Application`
without starting a server. `tusk init` creates `tusk.json` only; it does not
generate a PHP application skeleton.

| Path | Owner | Purpose |
| --- | --- | --- |
| `bootstrap/app.php` | Application | Compose providers, routes, middleware, and the application kernel. |
| `config/*.php` | Application | Application settings such as database and cache. `config.php` has no special runtime meaning. |
| `routes/*.php` | Application | Route definitions. |
| `public/index.php` | Application | Delegate HTTP requests to the bootstrap for conventional tooling; it is not the RoadRunner loop. |
| `tusk.json` | Engine/platform configuration | Ports, worker count, runtime limits, and other platform settings. |
| `composer.json` and lockfile | Composer | PHP dependencies, autoloading, and scripts. |
| `.tusk/runtime/worker.php` | Engine-generated state | Private RoadRunner worker; do not edit or commit it. |

`tusk start` validates the bootstrap, generates the worker inside `.tusk`, and
projects `php .tusk/runtime/worker.php` as RoadRunner's server command. The
worker loads Composer and the application once per PHP worker. RoadRunner owns
HTTP, Goridge, worker pooling, recycling, and process shutdown; the application
resets request-scoped services after every request. Startup fails on invalid
bootstrap, missing RoadRunner, or worker publication errors. The Engine does
not select an alternate request runtime.

## Generated worker platform limits

Windows uses strict handle-based ownership checks when publishing and
removing the generated worker. Linux supports publication only where the
filesystem provides the required atomic operations. Linux cleanup can
preserve a `.worker-quarantine-*` artifact and report an error if it cannot
prove safe removal; arrange exclusive access before handling that artifact.
Other Unix targets currently fail closed before creating the runtime worker.
The Engine never overwrites user-owned `bootstrap/`, `config/`, or `routes/`
files.

The Engine Dockerfile provides the Engine binary and PHP, not a project or a
root worker. Mount a modern application at `/app`; the image also needs a
RoadRunner executable available to `tusk start`.
