# Tusk Engine

The **Tusk Engine** is the Go control plane for the Tusk Framework and its RoadRunner runtime. It owns project configuration, RoadRunner lifecycle, readiness, diagnostics, and platform operations. RoadRunner owns HTTP, PHP workers, IPC, pooling, recycling, and request limits.

The embedded native HTTP/NDJSON server is a frozen migration-era implementation detail. New deployments use RoadRunner; the native path is not a second supported platform architecture.

## Features

- **Runtime Control**: Validates, starts, monitors, reloads, and stops RoadRunner without duplicating its worker pool.
- **Portable**: Manages the configured RoadRunner/PHP process boundary across supported development and deployment environments.
- **Unified CLI**: Provides project, lifecycle, diagnostics, and platform commands above the runtime.
- **Dual Config Support**: Works with both `tusk.json` and standard `composer.json` - use whichever you prefer!
- **Package Management**: Composer-backed convenience commands; Composer remains the dependency resolver and lockfile authority.
- **Project CLI**: The `tusk` binary handles project management, dependency commands, diagnostics, and framework commands.
- **Dynamic Config**: Automatically loads settings from `tusk.json` or `composer.json`.
- **Process Management**: Supervises the RoadRunner process and reports runtime failures.
- **Control API**: Optional, versioned health, readiness, metadata, and Prometheus endpoints for local operations.
- **Component model**: Activates versioned, replaceable service-invocation and resilience providers before serving traffic.

## Architecture

```mermaid
graph TD
    subgraph Engine ["Tusk Engine (Go control plane)"]
        Config["Config + lifecycle"] --> RR["RoadRunner child"]
        Control["Control API"] -.-> Config
    end

    RR --> Worker["PHP Workers (Tusk Framework)"]
    
    style Engine fill:#f9f9f9,stroke:#333,stroke-width:1px
    style Worker fill:#fff,stroke:#333,stroke-width:1px
```

## Installation

Download the latest binary for your platform from the [GitHub Releases](https://github.com/tusk-framework/tusk-engine/releases) page.

### Windows (PowerShell)
```powershell
# Download the latest release
$url = "https://github.com/tusk-framework/tusk-engine/releases/latest/download/tusk_Windows_x86_64.zip"
Invoke-WebRequest -Uri $url -OutFile tusk.zip
Expand-Archive tusk.zip -DestinationPath "$env:USERPROFILE\.tusk"

# Add to PATH (run once)
$env:PATH += ";$env:USERPROFILE\.tusk"
[Environment]::SetEnvironmentVariable("PATH", $env:PATH, "User")
```

### Ubuntu / macOS (Bash)
```bash
# Download the latest release
# Linux amd64:
curl -L https://github.com/tusk-framework/tusk-engine/releases/latest/download/tusk_Linux_x86_64.tar.gz | tar xz
# macOS Apple Silicon:
# curl -L https://github.com/tusk-framework/tusk-engine/releases/latest/download/tusk_Darwin_arm64.tar.gz | tar xz

mkdir -p ~/.tusk && mv tusk ~/.tusk/tusk

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH="$HOME/.tusk:$PATH"
```

> [!NOTE]
> After installation, restart your terminal (or run `source ~/.bashrc`) to ensure `tusk` is available on your PATH.

## Manual Build
```bash
go build -o tusk ./cmd/tusk
```

## Quick Start

### 1. Initialize Your Project
```bash
# Create a new tusk.json file
tusk init

# Or use your existing composer.json - tusk reads both!
```

### 2. Configure (Optional)
Create or edit `tusk.json` in your project root:
```json
{
    "port": 8080,
    "worker_count": 4,
    "php_binary": "php",
    "worker_command": "worker.php",
    "public_dir": "public",
    "timeout": 30,
    "max_body_bytes": 10485760,
    "max_upload_bytes": 10485760,
    "max_upload_files": 20,
    "runtime": {
        "status_address": "127.0.0.1:2114",
        "rpc_address": "tcp://127.0.0.1:6001",
        "startup_timeout": 30000000000,
        "probe_interval": 250000000
    },
    "control": {
        "enabled": false,
        "address": "127.0.0.1",
        "port": 9091
    },
    "scripts": {
        "dev": "tusk start",
        "test": "phpunit"
    }
}
```

Requests above the configured body or upload limits are rejected with HTTP 413. Static files are served only from `public/`; path traversal attempts are rejected. Scripts from `tusk.json` override scripts with the same name from `composer.json`, while non-conflicting scripts are merged.

### Component model

The Engine validates and activates configured components before starting
RoadRunner or the control server. Add component settings under
`"components"` in `tusk.json`; invalid names, fields, types, provider
configuration, or startup health checks fail before traffic is served. The
default registry provides the transport-free
`in-process-service-invocation` contract and the bounded
`default-resilience` provider. See [docs/components.md](docs/components.md)
for configuration, idempotency, retry, deadline, circuit-breaker, metadata,
and provider-substitution guidance.

### Toolchain diagnosis

The Engine can inspect the exact PHP, Composer, and RoadRunner executables
available to the project without changing the machine:

```bash
tusk doctor
tusk doctor --json
tusk toolchain list
tusk toolchain pin php@8.3
tusk setup --toolchain --offline
```

When `.tusk/toolchain.json` declares a relative executable path, the project
binary takes precedence over `PATH`. `tusk setup --toolchain` is the only
command that provisions tools: it loads the signed catalog at
`.tusk/toolchain.catalog.json`, installs only pinned tools without explicit
paths, and records managed paths only after verified installation. Add
`--offline` to require a digest-checked cache hit; missing, unsigned, or
expired catalogs fail before download. No executable is silently replaced and
the global `PATH` is never changed.

Release operators must follow the [official catalog release
runbook](release/README.md) for payload review, environment-only signing,
provenance, key rotation, and revocation. The checked-in payload is signing
input only; it is not a trusted runtime catalog.

**Or use composer.json** - tusk automatically reads scripts and configuration:
```json
{
    "name": "my/project",
    "require": {
        "php": "^8.0"
    },
    "scripts": {
        "dev": "tusk start",
        "test": "phpunit"
    }
}
```

> [!NOTE]
> If both `tusk.json` and `composer.json` exist, tusk.json takes priority but scripts from both are merged.

### Control API

The control API is disabled by default and does not change the public traffic server. Enable it explicitly for local health checks and observability:

```json
{
    "control": {
        "enabled": true,
        "address": "127.0.0.1",
        "port": 9091,
        "metrics_path": "/v1/metrics"
    },
    "runtime": {
        "metrics_address": "127.0.0.1:2112"
    }
}
```

When enabled, the engine exposes:

- `GET /v1/healthz` — process health; returns `200` during startup and graceful shutdown.
- `GET /v1/readyz` — RoadRunner readiness; returns `200` only after its status plugin reports an HTTP worker ready.
- `GET /v1/metadata` — safe engine and runtime metadata.
- `GET /v1/metrics` — authenticated composition of Engine and RoadRunner Prometheus metrics.

The default loopback binding does not require a token. If `address` is non-loopback, configure a non-empty `token`; every control endpoint then requires `Authorization: Bearer <token>`. Do not expose the control API publicly without a network policy and secret management appropriate for your deployment.

RoadRunner's Prometheus listener is internal and loopback-only at
`http://127.0.0.1:2112/metrics` by default. Tusk renders RoadRunner's
`http_metrics` middleware, which supplies bounded method/status/duration request
metrics plus worker state and queue-depth metrics, then composes that scrape
behind the authenticated Control API. The runtime metrics listener rejects
non-loopback addresses; use the Control API or an authenticated monitoring
gateway/local scrape agent when metrics must be collected remotely. The
Control API and its metrics route are disabled by default.

The legacy native runtime also publishes bounded `tusk_worker_starts_total`,
`tusk_worker_stops_total`, `tusk_worker_crashes_total`,
`tusk_worker_timeouts_total`, and `tusk_worker_queue_depth` collectors through
the control registry. RoadRunner remains the source of truth for the default
runtime's worker lifecycle and queue metrics, avoiding a second worker-pool
implementation in the Engine.

### Engine components

The Engine activates its versioned component registry before the control plane
or RoadRunner starts. Configure validated, transport-free providers under the
`components` object in `tusk.json`; see [Engine components](docs/components.md)
for the default service-invocation and resilience providers, bounded retry and
idempotency rules, provider substitution, and safe metadata behavior.

### 3. Manage Dependencies with Composer
```bash
# Install dependencies
tusk install

# Add a package
tusk add symfony/console

# Remove a package
tusk remove symfony/console

# Update dependencies
tusk update
```

### 4. Run Scripts
```bash
# Run any script defined in tusk.json or composer.json

# Explicit way (recommended for clarity)
tusk run dev
tusk run test

# Shorthand (backward compatible)
tusk dev
tusk test

# Scripts from both config files work seamlessly
```

> [!TIP]
> Use `tusk run <script>` for explicit script execution, or just `tusk <script>` as shorthand.
> Both work the same way, but `tusk run` makes it clear you're running a script.

### 5. Start RoadRunner
```bash
# Start the managed RoadRunner runtime
tusk start

# Or specify a custom worker file
tusk start custom-worker.php
```

> [!TIP]
> The Engine generates a private RoadRunner configuration below `.tusk/runtime`, starts `rr serve`, waits for `/ready?plugin=http`, and shuts RoadRunner down gracefully. Status and RPC sockets are loopback-only by default. If RoadRunner is missing or fails readiness, `tusk start` exits with a diagnostic and never falls back to the legacy native server.
>
> You can customize the worker file in two ways:
> 1. **Command-line**: `tusk start my-worker.php` (takes precedence)
> 2. **Config file**: Set `"worker_command": "my-worker.php"` in `tusk.json`

## Why Use Tusk with RoadRunner?

Tusk's RoadRunner control plane preserves the persistent PHP model while giving the runtime a single owner for HTTP and worker supervision:

### ⚡ Performance & Features
- **Stateful Workers**: Unlike `php -S` which creates a new process per request, RoadRunner maintains a pool of long-running PHP workers
- **State Management**: Workers keep state between requests - perfect for caching, connection pooling, and performance
- **Auto-Restart**: RoadRunner recycles workers according to its limits and supervision policy
- **Concurrent Requests**: RoadRunner handles the worker pool and concurrency
- **Production-Ready**: The same lifecycle contract is used in development and production

### 📝 Use Tusk Server in Scripts
Use `tusk start` or `tusk dev` to launch the managed RoadRunner runtime:

```json
{
  "scripts": {
    "dev": "tusk start"
  }
}
```

The `tusk dev` command is an alias for `tusk start`.

## Composer-backed Package Commands

Tusk provides a unified CLI around the PHP runtime while keeping Composer as the source of truth for dependency resolution:

### 🔄 Automatic Config Detection
- Reads from `tusk.json` (custom Tusk config)
- Falls back to `composer.json` (standard PHP)
- Merges scripts from both if both exist
- Priority: `tusk.json` > `composer.json`
- Reads the Composer metadata and scripts needed by the engine without reimplementing Composer's dependency solver

### 📦 Composer-backed Dependency Commands
These convenience commands delegate dependency work to Composer. Composer
remains the dependency solver and `composer.lock` authority; setup may provide
the Composer PHAR and its resolved PHP interpreter:

```bash
tusk install              # Install all dependencies
tusk add vendor/package   # Add a new package
tusk remove vendor/package # Remove a package
tusk update               # Update all dependencies
tusk update vendor/package # Update specific package
```

### 🚀 Script Runner
Run any script from `tusk.json` or `composer.json`:

```bash
# Explicit (recommended)
tusk run dev    # Run your dev script
tusk run test   # Run your test script
tusk run build  # Run your build script

# Shorthand (backward compatible)
tusk dev    # Same as 'tusk run dev'
tusk test   # Run your test script
tusk build  # Run your build script
```

### 🎯 Unified Interface
Everything through one command:
- Server management: `tusk start`, `tusk dev`
- Package management: `tusk install`, `tusk add`, `tusk update`
- Script execution: `tusk run <script>` or `tusk <script>`
- Framework commands: `tusk make:controller` (proxied to PHP)

### 📋 Composer Integration
Tusk reads the relevant `composer.json` fields, including:
- Package metadata: name, description, version, type, keywords
- Licensing: license, authors, homepage
- Dependencies for display/configuration: require, require-dev, conflict, replace, provide, suggest
- Autoloading: autoload, autoload-dev (PSR-4, PSR-0, classmap, files)
- Configuration: config, extra, bin, repositories
- Scripts, including array-style scripts with proper execution

Dependency resolution and lockfile generation remain Composer responsibilities.

## Runtime boundary
RoadRunner owns the HTTP and PHP worker protocol. The Engine does not act as a reverse proxy or duplicate RoadRunner's pool. The legacy NDJSON implementation remains only for migration and is not the platform request path.
