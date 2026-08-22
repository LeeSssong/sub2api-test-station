# Task 2 report — isolated admin lab Compose image strategy

## Scope

Added an explicit local source build contract for both `admin-lab-api` and
`admin-lab-worker`. Each service now builds from `../upstream/sub2api/Dockerfile`
and retains the deterministic `sub2api-admin-lab:local` image tag. The Compose
contract test asserts that an image-only or registry-only declaration cannot
silently replace the checked-out Sub2API source.

## RED → GREEN evidence

- RED (before implementation): `bash tests/admin_lab/compose_contract_test.sh`
  failed with `missing compose isolation contract: dockerfile: Dockerfile`.
- GREEN: after adding `build.context`/`build.dockerfile` to both services, the
  same contract passes.

## Verification

- `docker compose --project-name sub2api-admin-lab --env-file infra/.env.admin-lab.example -f infra/compose.admin-lab.yaml config --quiet` — PASS.
- `bash tests/admin_lab/compose_contract_test.sh` — PASS.
- `git diff --check` — PASS.
- `docker build --file upstream/sub2api/Dockerfile --tag sub2api-admin-lab:local upstream/sub2api` — PASS (local image built; no services started).

## Changes

- `infra/compose.admin-lab.yaml`
- `tests/admin_lab/compose_contract_test.sh`

## Risks / unverified

- The Compose file intentionally keeps `admin-lab-gateway` on the external
  `sub2api_default` network for the later Caddy integration task; this task did
  not create or modify that network and did not start the stack.
- No production database, Redis, network, deployment, or host service was
  touched.

Status: `READY_FOR_ROOT_REVIEW`
