# Cross-repository skeleton smoke contract

The Engine skeleton smoke test consumes the Framework from the published
`main` ref configured in `.github/workflows/test.yml` and verifies that the
resolved checkout contains the modern generator contract.

Publication order is deliberate:

1. Publish the Framework generator contract on `main`.
2. Confirm that the smoke workflow resolves and validates that ref.
3. Run the Engine smoke workflow against the resolved checkout.

The workflow fetches the named ref and validates the required generator files
before checking out the Framework. If the ref is absent or the contract is
missing, CI fails with an actionable error.

The smoke test therefore exercises the same contract documented for runtime:
`bootstrap/app.php` composes the application, the Engine generates
`.tusk/runtime/worker.php`, and RoadRunner owns HTTP, Goridge, PHP workers,
pooling, and shutdown. The Engine owns lifecycle and control-plane behavior;
the modern contract is the only supported project path.
