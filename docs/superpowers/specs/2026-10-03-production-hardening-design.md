# Archived: Tusk Engine production hardening design

> Status: Obsolete. This historical design is retained for traceability and
> is not repository guidance.

This design predates the current RoadRunner architecture and must not be used
to select a worker path or assign runtime ownership. It referred to a
repository-owned worker, Docker worker copying, and an Engine-side NDJSON
transport that are no longer the supported production contract.

The current contract is:

- `bootstrap/app.php` composes and returns the application.
- The Engine generates `.tusk/runtime/worker.php` for each project.
- RoadRunner owns HTTP, Goridge, PHP workers, pooling, recycling, and process
  shutdown.
- The Engine owns lifecycle and control-plane behavior.
- Legacy migration is explicit and never selected implicitly.

See [`docs/guides/project-runtime.md`](../../guides/project-runtime.md) and
[`2026-10-04-roadrunner-control-plane-design.md`](2026-10-04-roadrunner-control-plane-design.md)
for active guidance.
