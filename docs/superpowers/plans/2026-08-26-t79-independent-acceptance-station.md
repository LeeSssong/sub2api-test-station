# T79 独立准生产验收站 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fixed, independent, real-flow Sub2API acceptance station that administrators can deploy serially after local validation and before manually promoting a candidate to production.

**Architecture:** Create a dedicated `sub2api-acceptance` Compose topology with a single API, worker, detector, PostgreSQL, Redis and Caddy. Add a local controller and host executor that require a 0600 operator env file, reject production/mock identities, preserve named volumes, bootstrap the native admin-only settings, and never invoke the production deployment path.

**Tech Stack:** Docker Compose, Caddy, Bash, SSH/SCP, PostgreSQL `psql`, existing Sub2API image/Dockerfile, shell contract tests.

**Spec:** `docs/superpowers/specs/2026-08-26-t79-independent-acceptance-station-design.md`

## Global Constraints

- Project name is exactly `sub2api-acceptance`; network is exactly `sub2api-acceptance-network`.
- No service, env default, deploy path, domain, Caddy route, Docker network, volume or credential may reuse production identifiers.
- Deployment is serial and single-instance: no blue-green, no ephemeral environments, no auto-promotion.
- Acceptance must use real flow declarations and must reject `mock`, `mock-upstream` and `lab-outbox`.
- Bootstrap must set `backend_mode_enabled=true` and `registration_enabled=false` only in the acceptance database.
- Failed deployments preserve acceptance named volumes and restore prior compose/Caddy/env where available.
- Do not add GitHub Actions or change production release scripts/Caddy.

---

### Task 1: Add the acceptance topology and its configuration contract

**Files:**
- Create: `infra/compose.acceptance.yaml`
- Create: `infra/Caddyfile.acceptance`
- Create: `infra/.env.acceptance.example`
- Test: `tests/acceptance_station/compose_contract_test.sh`

**Interfaces:**
- Consumes: `ACCEPTANCE_IMAGE`, `ACCEPTANCE_SITE_ADDRESS`, independent DB/Redis/admin secret variables from the env file.
- Produces: Compose project `sub2api-acceptance`, six healthy services, named volumes `sub2api-acceptance-*`, network `sub2api-acceptance-network`, and profile `bootstrap`.

- [ ] **Step 1: Write the failing topology contract**

Create `tests/acceptance_station/compose_contract_test.sh` with assertions that the topology files exist and that the rendered Compose configuration contains the independent project/network/volumes and native service names, while no raw `mock`, `mock-upstream`, `lab-outbox`, `sub2api_default`, `sub2api-blue` or `sub2api-green` string is allowed:

```bash
docker compose --project-name sub2api-acceptance \
  --env-file infra/.env.acceptance.example \
  -f infra/compose.acceptance.yaml config --quiet

grep -Fq 'name: sub2api-acceptance' infra/compose.acceptance.yaml
grep -Fq 'sub2api-acceptance-network' infra/compose.acceptance.yaml
grep -Fq 'acceptance-bootstrap' infra/compose.acceptance.yaml
if rg -n 'mock-upstream|PAYMENT_PROVIDER: mock|lab-outbox|sub2api_default|sub2api-blue|sub2api-green' \
  infra/compose.acceptance.yaml infra/Caddyfile.acceptance; then exit 1; fi
```

- [ ] **Step 2: Run the contract to verify RED**

Run: `bash tests/acceptance_station/compose_contract_test.sh`

Expected: fail because `infra/compose.acceptance.yaml` does not exist.

- [ ] **Step 3: Implement the minimal independent topology**

Create Compose services `acceptance-api`, `acceptance-worker`, `acceptance-detector`, `acceptance-postgres`, `acceptance-redis`, `acceptance-caddy`, and profile-only `acceptance-bootstrap`. Wire API/worker to `acceptance-postgres`, `acceptance-redis`, and `acceptance-detector`; publish only Caddy `80:80`/`443:443`; use named volumes and the internal network. Make bootstrap execute the idempotent native settings upsert for `backend_mode_enabled=true` and `registration_enabled=false`.

Use a minimal independent Caddyfile with a 15-minute upstream response timeout and no `/admin/lab/` route. Add only example values to `.env.acceptance.example`, including non-production domain/root/project/network, intentionally invalid example secrets, real-flow declaration values, and `ACCEPTANCE_REAL_FLOW_ACK=I_UNDERSTAND_REAL_CHARGES`.

- [ ] **Step 4: Run the contract to verify GREEN**

Run: `bash tests/acceptance_station/compose_contract_test.sh`

Expected: pass and print `acceptance station compose contract: PASS`.

- [ ] **Step 5: Commit**

```bash
git add infra/compose.acceptance.yaml infra/Caddyfile.acceptance infra/.env.acceptance.example \
  tests/acceptance_station/compose_contract_test.sh
git commit -m "feat: add independent acceptance topology"
```

### Task 2: Add the local acceptance release controller

**Files:**
- Create: `ops/release-sub2api-acceptance.sh`
- Test: `tests/acceptance_station/release_delivery_contract_test.sh`

**Interfaces:**
- Consumes: clean Git worktree, `ACCEPTANCE_ENV_FILE`, SSH transport variables and `ops/deploy-sub2api-acceptance-host.sh`.
- Produces: a Linux/amd64 image archive plus exact source commit/tree and a remote invocation of the acceptance host executor.

- [ ] **Step 1: Write the failing release controller contract**

Create `tests/acceptance_station/release_delivery_contract_test.sh` to require the new controller and executor, required refusal strings, strict 0600 checks, image archive SHA-256, `docker buildx build --load`, SSH/SCP transfer, and an explicit ban on production release script names:

```bash
for file in ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh; do
  [[ -x "$file" ]] || fail "$file is missing or not executable"
done
grep -Fq 'I_UNDERSTAND_REAL_CHARGES' ops/release-sub2api-acceptance.sh
grep -Fq 'api.xingqiaolab.top' ops/release-sub2api-acceptance.sh
grep -Fq 'sub2api_default' ops/release-sub2api-acceptance.sh
! rg -n 'release-sub2api-blue-green|deploy-sub2api-blue-green|release-admin-lab' \
  ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh
```

- [ ] **Step 2: Run the contract to verify RED**

Run: `bash tests/acceptance_station/release_delivery_contract_test.sh`

Expected: fail because the controller and executor do not exist.

- [ ] **Step 3: Implement the local controller**

Implement env-driven `ops/release-sub2api-acceptance.sh` with these checks before any SSH/SCP:

```bash
[[ -z "$(git status --porcelain)" ]] || fail 'worktree is dirty'
[[ "$(mode_of "$acceptance_env")" == 600 && ! -L "$acceptance_env" ]] || fail 'ACCEPTANCE_ENV_FILE must be a 0600 non-symlink file'
[[ "$real_flow_ack" == I_UNDERSTAND_REAL_CHARGES ]] || fail 'ACCEPTANCE_REAL_FLOW_ACK is required'
case "$site_address:$deploy_root:$project_name:$network_name" in
  *api.xingqiaolab.top*|*/opt/sub2api/production*|*sub2api_default*|*':sub2api:'*) fail 'production identity is forbidden' ;;
esac
case "$payment_provider:$upstream_provider:$notification_transport" in
  *mock*|*lab-outbox*) fail 'mock flow is forbidden' ;;
esac
```

Build one Linux/amd64 image from `upstream/sub2api`, save it to a temporary tar, checksum it, package the acceptance Compose/Caddy/env, and transfer files to an SSH-created temporary directory. Invoke only the dedicated host executor through `sudo -n bash -s`; do not copy the acceptance env into Git or print its contents.

- [ ] **Step 4: Run the controller contract to verify GREEN**

Run: `bash tests/acceptance_station/release_delivery_contract_test.sh`

Expected: pass up to the host-executor-specific assertions introduced in Task 3.

### Task 3: Add the host executor, admin-only bootstrap, health checks, and rollback

**Files:**
- Create: `ops/deploy-sub2api-acceptance-host.sh`
- Modify: `tests/acceptance_station/release_delivery_contract_test.sh`
- Create: `tests/acceptance_station/auth_mode_contract_test.sh`

**Interfaces:**
- Consumes: staged acceptance bundle, image archive, 0600 env, source commit/tree and `--deploy-root`.
- Produces: root-owned acceptance runtime files, preserved acceptance volumes, and JSON `{result:"succeeded",downtime_required:false,...}`.

- [ ] **Step 1: Extend failing contracts**

Add host-executor and bootstrap assertions:

```bash
grep -Fq 'docker compose --project-name sub2api-acceptance' ops/deploy-sub2api-acceptance-host.sh
grep -Fq 'acceptance-bootstrap' ops/deploy-sub2api-acceptance-host.sh
grep -Fq "('backend_mode_enabled', 'true')" infra/compose.acceptance.yaml
grep -Fq "('registration_enabled', 'false')" infra/compose.acceptance.yaml
grep -Fq 'ON CONFLICT (key) DO UPDATE' infra/compose.acceptance.yaml
grep -Fq 'rollback' ops/deploy-sub2api-acceptance-host.sh
```

- [ ] **Step 2: Run both contracts to verify RED**

Run:

```bash
bash tests/acceptance_station/release_delivery_contract_test.sh
bash tests/acceptance_station/auth_mode_contract_test.sh
```

Expected: fail because the host executor has not been implemented.

- [ ] **Step 3: Implement the dedicated host executor**

Implement a `set -euo pipefail` executor that validates all argument paths are under the acceptance staging root, validates checksums and env mode, validates non-production identity again, extracts the bundle into an `mktemp` staging directory, and installs only under the acceptance deploy root.

Keep a previous runtime copy before replacement. Rollback restores previous Compose/Caddy/env and runs the prior project with `up -d --wait` when prior files existed; it never removes volumes or runs a production Compose command.

After `docker load`, update the staged env with the exact image tag, then run:

```bash
docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" \
  -f "$deploy_root/compose.acceptance.yaml" up -d --wait --no-build
docker compose --project-name sub2api-acceptance --env-file "$deploy_root/.env" \
  -f "$deploy_root/compose.acceptance.yaml" --profile bootstrap run --rm acceptance-bootstrap
```

Probe `https://$ACCEPTANCE_SITE_ADDRESS/health` and `/auth/login`, verify six long-running services exist and are healthy, and emit only redacted JSON success data.

- [ ] **Step 4: Run contracts to verify GREEN**

Run:

```bash
bash tests/acceptance_station/compose_contract_test.sh
bash tests/acceptance_station/release_delivery_contract_test.sh
bash tests/acceptance_station/auth_mode_contract_test.sh
bash -n ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh
git diff --check
```

Expected: all pass with no diff whitespace error.

- [ ] **Step 5: Commit**

```bash
git add ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh \
  tests/acceptance_station/release_delivery_contract_test.sh \
  tests/acceptance_station/auth_mode_contract_test.sh
git commit -m "feat: add independent acceptance release chain"
```

### Task 4: Document manual acceptance, retirement boundary, and final verification

**Files:**
- Create: `docs/runbooks/sub2api-acceptance-station.md`
- Modify: `tests/acceptance_station/release_delivery_contract_test.sh`

**Interfaces:**
- Consumes: completed T79 files and operator-installed independent host/domain/credentials.
- Produces: a manual serial operating procedure with no auto-promotion.

- [ ] **Step 1: Write the failing documentation assertion**

Add to the release delivery contract:

```bash
runbook=docs/runbooks/sub2api-acceptance-station.md
[[ -f "$runbook" ]] || fail 'acceptance runbook is missing'
grep -Fq '本地直接验证 -> 部署验收站 -> 管理员真实验收 -> 人工合入 main -> 人工部署主站' "$runbook"
grep -Fq '不自动晋级' "$runbook"
grep -Fq '/admin/lab/' "$runbook"
```

- [ ] **Step 2: Run the contract to verify RED**

Run: `bash tests/acceptance_station/release_delivery_contract_test.sh`

Expected: fail because the runbook does not exist.

- [ ] **Step 3: Write the runbook**

Document prerequisite values, host-side DNS/firewall restriction, env file creation at 0600, exact release command, post-deploy checks, real payment/upstream acceptance checklist, manual production promotion boundary, recovery command and old `/admin/lab/` retirement steps. State explicitly that a successful deployment is not functional acceptance and cannot auto-deploy production.

- [ ] **Step 4: Run final scoped verification**

Run:

```bash
bash tests/acceptance_station/compose_contract_test.sh
bash tests/acceptance_station/release_delivery_contract_test.sh
bash tests/acceptance_station/auth_mode_contract_test.sh
docker compose --project-name sub2api-acceptance --env-file infra/.env.acceptance.example \
  -f infra/compose.acceptance.yaml config --quiet
bash -n ops/release-sub2api-acceptance.sh ops/deploy-sub2api-acceptance-host.sh
git diff --check
```

Expected: all pass. Do not perform a real acceptance-host deployment without operator-provided independent host/domain/credentials.

- [ ] **Step 5: Commit**

```bash
git add docs/runbooks/sub2api-acceptance-station.md tests/acceptance_station/release_delivery_contract_test.sh
git commit -m "docs: add acceptance station runbook"
```

## Plan Self-Review

- Spec coverage: Tasks 1–3 implement independent topology, real-flow rejection, admin-only bootstrap, preservation/rollback and release controls. Task 4 documents manual acceptance and lab retirement.
- Placeholder scan: no TODO/TBD/implicit test steps.
- Interface consistency: controller emits staged bundle/archive/env consumed by host executor; Compose profile name is consistently `acceptance-bootstrap`; all topology names use `sub2api-acceptance`.

