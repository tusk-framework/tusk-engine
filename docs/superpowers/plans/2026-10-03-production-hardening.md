# Archived: Tusk Engine production hardening plan

> Status: Obsolete. This historical plan is retained for traceability and is
> not repository guidance.

The plan described an earlier runtime design with a repository-owned worker,
Docker worker copying, and Engine-side NDJSON worker ownership. That design was
superseded by the RoadRunner control-plane work.

Use the current contract in
[`docs/guides/project-runtime.md`](../../guides/project-runtime.md) and
[`docs/superpowers/specs/2026-10-04-roadrunner-control-plane-design.md`](../specs/2026-10-04-roadrunner-control-plane-design.md):

- `bootstrap/app.php` is application composition.
- The Engine generates `.tusk/runtime/worker.php`.
- RoadRunner owns HTTP, Goridge, PHP workers, pooling, recycling, and process
  shutdown.
- The Engine owns lifecycle and control-plane behavior.
- Legacy migration is explicit; there is no silent native or legacy-worker
  fallback.
