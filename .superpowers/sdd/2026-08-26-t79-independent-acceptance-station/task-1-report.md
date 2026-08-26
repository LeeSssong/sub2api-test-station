# T79 Task 1 Report — independent acceptance topology

## Status

Implemented and committed. The independent acceptance topology is ready for the
dedicated release-controller task; it has not been installed on any host.

## Changes

- Added `infra/compose.acceptance.yaml` with Compose project
  `sub2api-acceptance`, seven declared services (six long-running healthy
  services plus profile-only `acceptance-bootstrap`), fixed named acceptance
  volumes, and the internal `sub2api-acceptance-network`.
- Added `infra/Caddyfile.acceptance` with the dedicated acceptance site address,
  a 15-minute upstream response timeout, and only `acceptance-api:8080` as the
  upstream.
- Added `infra/.env.acceptance.example` with deliberately invalid example
  values, independent identities, real-flow declarations, and the explicit
  real-charge acknowledgement.
- Added `tests/acceptance_station/compose_contract_test.sh`, which checks
  topology isolation, native service names, bootstrap SQL, timeout, and absence
  of mock/lab or production blue-green identifiers.

## Test evidence

RED, before topology implementation:

```text
acceptance station compose contract: FAIL: infra/compose.acceptance.yaml is missing
```

GREEN, after implementation:

```text
bash tests/acceptance_station/compose_contract_test.sh
acceptance station compose contract: PASS
```

Also passed:

```text
docker compose --project-name sub2api-acceptance --env-file infra/.env.acceptance.example -f infra/compose.acceptance.yaml config --quiet
git diff --check
```

## Configuration and migration changes

- New acceptance-only Compose, Caddy, and example environment configuration.
- No application/database migration.
- No production configuration, release scripts, deploy state, queue, or project
  progress records were modified.

## Deployment and downtime

- `downtime_required=false` for this task's intended acceptance-host deployment
  model: it does not stop or modify production. The fixed acceptance instance
  may restart during its own serial update.
- No actual deployment was attempted because no independent operator host,
  DNS/firewall boundary, or 0600 runtime environment file has been provided.

## Remaining risks / unverified items

- Real payment, upstream, notification, recharge, and consumption flows remain
  intentionally unverified until an operator supplies independent credentials
  and performs administrator acceptance.
- The later dedicated release controller and host executor still need to enforce
  the environment-file, non-production identity, staging, rollback, bootstrap,
  and health-check runtime controls.
