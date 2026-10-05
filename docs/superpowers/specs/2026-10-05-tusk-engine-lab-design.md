# Tusk Engine Lab Design

## Status

Proposed design for an external local integration laboratory.

## Intent

Create a disposable but repeatable local project that validates the complete
contract of Tusk Engine from a user's perspective. The laboratory must test
the published release and a locally built Engine without placing generated
application state, downloaded toolchains, or test-only files in the Engine
repository.

The laboratory is not a second implementation of the Engine. It is an
acceptance harness for the public CLI, the project contract, the signed
toolchain catalog, and the RoadRunner control-plane lifecycle.

## Location and execution

The project lives outside the repositories at:

`C:\Users\wende\Projects\tusk\tusk-engine-lab`

WSL is the preferred execution environment for repeatable shell scripts. The
Windows host remains a supported target because the official Windows release
and Windows toolchain artifacts are part of the release audit. The lab must
therefore avoid assumptions that only work in Bash or only work in PowerShell.

The first implementation targets Windows/WSL and keeps the test boundaries
portable to Linux later. Network-dependent tests are explicit; offline tests
use a prepared cache and must never silently download artifacts.

## Test modes

### Local source mode

Build the Engine from a selected local checkout, then run the lab against that
binary. This mode catches regressions before a release and may use a local
Framework checkout when the cross-repository contract requires it.

### Published release mode

Download a pinned Engine release, its signed catalog, and the matching
platform artifacts. This mode validates the user-facing distribution rather
than the developer checkout. The release tag and artifact digests are recorded
in the test result.

Both modes use the same application fixture and assertions.

## Fixture application

The fixture is a minimal modern Tusk application with these owned files:

- `bootstrap/app.php`, returning a configured `Tusk\\Foundation\\Application`.
- `routes/web.php`, defining deterministic health and request endpoints.
- `config/`, containing non-secret application settings.
- `public/index.php`, for conventional PHP tooling only.
- `composer.json` and `composer.lock`, with the smallest supported Framework
  dependency set.
- `tusk.json`, containing explicit runtime and control-plane settings.

The fixture must not contain a root `worker.php`; the Engine must generate
`.tusk/runtime/worker.php` and RoadRunner must own the worker lifecycle.

The HTTP contract should expose a stable response containing the request path,
the application identity, and a request correlation value. A separate health
route must not require application dependencies that are absent from the
fixture.

## Validation stages

The `run-all` entry point executes these stages independently and writes a
machine-readable result plus human-readable logs:

1. **CLI contract** — `help`, `init`, `doctor`, toolchain list and pinning.
2. **Trust and toolchain** — signed catalog verification, exact pinning,
   digest-checked provisioning, cache reuse, and offline refusal when a cache
   entry is missing.
3. **PHP project** — Composer install, autoload generation, and framework
   bootstrap validation.
4. **Runtime startup** — Engine starts RoadRunner, publishes the generated
   worker, and waits for readiness without a duplicate native server.
5. **HTTP behavior** — deterministic application request, health, readiness,
   and expected status/error responses.
6. **Control plane** — versioned health, readiness, metadata, and Prometheus
   endpoints when enabled in `tusk.json`; metadata must not expose secrets or
   component configuration.
7. **Components** — first-party invocation and resilience configuration are
   activated before serving traffic, and invalid component configuration fails
   startup clearly.
8. **Lifecycle** — logs are captured, reload is graceful, requests drain, and
   stop leaves no Engine-owned process behind.
9. **Failure modes** — invalid bootstrap, absent catalog, unpinned tools,
   unavailable RoadRunner, port collision, and malformed configuration produce
   non-zero, actionable failures.
10. **Repeatability** — cleanup removes only the lab's temporary state, and a
    second run produces the same assertions without manual repair.

## Harness layout

```text
tusk-engine-lab/
├── app/
│   ├── bootstrap/app.php
│   ├── config/
│   ├── public/index.php
│   ├── routes/
│   ├── composer.json
│   └── tusk.json
├── scripts/
│   ├── common.sh
│   ├── test-cli.sh
│   ├── test-toolchain.sh
│   ├── test-runtime.sh
│   ├── test-control-api.sh
│   ├── test-failure-modes.sh
│   └── run-all.sh
├── results/
└── README.md
```

Scripts must use strict error handling, dynamically selected ports, bounded
startup/request timeouts, process-group cleanup, and traps that run on success
and failure. Each stage reports `pass`, `fail`, or `skip` with the command,
exit code, relevant paths, and a short diagnostic. Secrets and private signing
material must never be copied into the lab.

## Release and source assertions

Published mode must prove all of the following before runtime tests begin:

- the release is not a draft or prerelease;
- the catalog signature verifies against the configured public trust anchor;
- catalog and provenance attestations belong to the expected release workflow
  and tag;
- every selected artifact matches its catalog digest;
- the downloaded Engine reports the expected release behavior.

Local source mode must record the Engine commit and Framework commit used by
the run. It must not rewrite the Engine repository or rely on uncommitted
generated files from another test run.

## Success criteria

The lab is successful when:

- both modes pass the shared fixture and lifecycle assertions;
- the full clean setup provisions PHP, Composer, and RoadRunner from the
  trusted catalog;
- a real HTTP request is served through RoadRunner-managed PHP workers;
- control-plane endpoints and lifecycle operations are observable;
- negative tests fail for the intended reason and do not leave processes or
  generated state outside the lab;
- the run can be repeated from a clean directory with one command;
- README instructions are sufficient for another developer to reproduce the
  run on Windows/WSL.

## Deliberate non-goals

- replacing the Engine's unit and package tests;
- benchmarking or capacity certification;
- testing every PHP framework feature;
- modifying GitHub releases, repository settings, or signing credentials;
- introducing Docker or a second runtime abstraction before the local contract
  is stable.
