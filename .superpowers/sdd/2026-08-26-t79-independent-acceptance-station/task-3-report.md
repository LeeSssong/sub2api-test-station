# T79 Task 3 implementation report

## Scope

Implemented the acceptance-host executor, native administrator-only bootstrap
contract, health verification, and non-destructive acceptance rollback. This
task changes only the T79 worktree's acceptance scripts and focused contracts;
it does not change root `main`, global queue/progress files, production Caddy,
production release scripts, or any deployment target.

## TDD evidence

1. Extended `tests/acceptance_station/release_delivery_contract_test.sh` and
   added `tests/acceptance_station/auth_mode_contract_test.sh` before creating
   the host executor.
2. Ran both focused contracts while the executor was absent. They failed as
   intended with `FAIL: ops/deploy-sub2api-acceptance-host.sh is missing` and
   `FAIL: acceptance host executor is missing or not executable`.
3. Added the executable `ops/deploy-sub2api-acceptance-host.sh`, then reran the
   contracts to green.

## Delivered behavior

- Requires root invocation and validates every staged file as a regular,
  non-symlink path contained in the private acceptance staging root. It checks
  0700 staging permissions, 0600 environment permissions, image SHA-256, and
  exact source commit/tree formats.
- Parses only literal `ACCEPTANCE_*` values; requires the dedicated project,
  network and matching acceptance deploy root; rejects production identities,
  mock/lab flows and a missing real-charge acknowledgement.
- Extracts runtime input into a private `mktemp` directory, loads the staged
  image, replaces `ACCEPTANCE_IMAGE` with that exact loaded image tag, and
  installs root-owned `compose.acceptance.yaml`, `Caddyfile.acceptance`, and
  `.env` only below the independent acceptance deploy root.
- Starts exactly `sub2api-acceptance` with `up -d --wait --no-build`, runs
  `acceptance-bootstrap`, and verifies the native `backend_mode_enabled=true`
  and `registration_enabled=false` settings in the acceptance PostgreSQL
  container.
- Verifies six long-running service health states and probes HTTPS `/health`
  and `/auth/login` using an explicit loopback host resolution for the
  configured acceptance virtual host.
- Preserves named volumes. On a failed replacement, restores the previous
  Compose/Caddy/env trio and reruns the previous acceptance project with
  `up -d --wait --no-build`; first-install failure only stops the acceptance
  stack. No production Compose command, volume removal, database reset or
  production release script is used.
- Emits redacted success JSON with `result:"succeeded"` and
  `downtime_required:false`; it does not print environment secrets.

## Verification

- `bash tests/acceptance_station/compose_contract_test.sh` — PASS
- `bash tests/acceptance_station/release_delivery_contract_test.sh` — PASS
- `bash tests/acceptance_station/auth_mode_contract_test.sh` — PASS
- `bash -n ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh` — PASS
- `git diff --check` — PASS

## Remaining external validation

No host deployment was attempted. An operator must still provide an independent
host, DNS/firewall restriction, real payment/upstream/notification credentials,
and 0600 SSH/environment files. A successful host deploy only makes the
acceptance instance ready for administrator real-flow acceptance; it does not
automatically promote any candidate to production.
