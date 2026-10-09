# CI and Docker

Pin tool versions in the project manifest and install Composer dependencies
from the committed lockfile in CI. Run project tests, `composer
check-platform-reqs --no-dev` for production requirements, and the Engine's
runtime smoke before publishing. The exact managed PHP targets available today
are reported by the official catalog and `tusk doctor`; do not assume a
cross-platform bundle exists until it is published and tested.

The current repository CI includes a generated Framework skeleton smoke that
uses PHP, Composer, and RoadRunner on its runner. It is not yet proof that a
clean host without PHP or Composer can use the Engine. A sanitized
no-global-PHP/Composer matrix is planned as part of managed-runtime delivery.

## Docker profile

Keep PHP, its native extensions, Composer dependencies, and RoadRunner in the
container image or mounted application environment. The Engine Docker image
provides the Engine and PHP, but not a complete application or a RoadRunner
binary; mount the application at `/app` and ensure RoadRunner is available to
`tusk start`. Avoid installing or downloading host-side tools from container
startup.

Use a pinned base image and runtime versions, install from `composer.lock`,
and run the same `composer check-platform-reqs --no-dev` check used in
production. Do not treat a successful local `config.platform` solve as proof
that the container has the required extensions.
