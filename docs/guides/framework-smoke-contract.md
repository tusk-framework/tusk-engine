# Cross-repository skeleton smoke contract

The Engine skeleton smoke test consumes the Framework from the coordinated
branch `codex/tusk-bootstrap` at the exact commit recorded in
`.github/workflows/test.yml` as `FRAMEWORK_SHA`.

Publication order is deliberate:

1. Commit and publish the Framework branch `codex/tusk-bootstrap`.
2. Confirm that the published branch resolves to `FRAMEWORK_SHA`.
3. Publish the Engine change that contains the matching branch and SHA.

The workflow fetches the named branch and verifies its resolved tip before
checking out the Framework. If the branch is absent or has moved, CI fails with
an actionable error. It does not fetch a local-only object and does not fall
back to the legacy Framework.

The smoke test therefore exercises the same contract documented for runtime:
`bootstrap/app.php` composes the application, the Engine generates
`.tusk/runtime/worker.php`, and RoadRunner owns HTTP, Goridge, PHP workers,
pooling, and shutdown. The Engine owns lifecycle and control-plane behavior;
legacy migration remains explicit.
