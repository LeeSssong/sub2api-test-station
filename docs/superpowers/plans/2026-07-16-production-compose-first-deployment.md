# Production Compose First Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use subagents unless the user explicitly authorizes delegation.

**Goal:** Replace the verified Caddy-only bootstrap with a healthy Sub2API, PostgreSQL, Redis, and Caddy production stack on the existing server without placing any API Key in files, chat, or shell history.

**Architecture:** Reuse the existing `sub2api` Compose project and Caddy certificate volumes. Generate all application and datastore secrets on the server into a mode-600 environment file, set only non-secret deployment values, validate before switching, then replace bootstrap Caddy with the full stack. If the full stack fails acceptance, stop it without deleting volumes and restore the bootstrap service.

**Tech Stack:** Ubuntu 24.04 LTS, Docker Engine 29.6.1, Docker Compose v5.3.1, Sub2API v0.1.161 (initial baseline v0.1.155), PostgreSQL 18 Alpine, Redis 8 Alpine, Caddy 2.10.2 Alpine, Bash, curl, OpenSSL.

## Global Constraints

- Do not read or output the existing local `infra/.env`.
- Do not use the API Key previously exposed in chat; it has been revoked.
- Do not request or place a replacement upstream Key in chat, Git, deployment files, shell history, or project documentation.
- Keep the temporary hostname and instance IPv4 runtime-only; do not persist either in project documentation.
- The temporary hostname remains operator-only and must not be shared with external testers.
- Only Caddy may publish host ports 80 and 443; PostgreSQL 5432, Redis 6379, and Sub2API 8080 must remain private.
- Do not delete production volumes during deployment or rollback.
- The repository has no commit history; do not create an implicit initial commit, branch, or worktree.

---

### Task 1: Local and remote preflight

**Files:**
- Verify: `infra/compose.yaml`
- Verify: `infra/Caddyfile`
- Verify: `infra/.env.example`
- Verify: `ops/generate-env.sh`
- Test: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: runtime-only `SERVER`, `SITE_ADDRESS`, and `SSH_IDENTITY` shell values.
- Produces: proof that local contracts pass and the server can resolve/reach the upstream host with sufficient disk, memory, and Docker health.

- [x] **Step 1: Run the local infrastructure contract**

Run: `tests/infra/validate-baseline.sh`

Expected: `PASS: infrastructure baseline contracts`.

- [x] **Step 2: Verify current remote isolation and capacity**

Use the dedicated SSH identity and require: only `sub2api-caddy-bootstrap-1` is running, at least 4 GiB free disk is available under `/opt`, swap is active, `vm.overcommit_memory=1`, and Docker is healthy.

- [x] **Step 3: Verify upstream network reachability without authentication**

From the server, request `https://aliuapi.top/v1/models` without an authorization header.

Expected: TLS verification succeeds and the server returns HTTP 401 with `API_KEY_REQUIRED`. Do not send an API Key.

### Task 2: Generate server-side configuration and switch stacks

**Files:**
- Remote create: `/opt/sub2api/production/compose.yaml`
- Remote create: `/opt/sub2api/production/Caddyfile`
- Remote create: `/opt/sub2api/production/.env`
- Remote temporary: `/opt/sub2api/production/.env.example`
- Remote temporary: `/opt/sub2api/production/generate-env.sh`

**Interfaces:**
- Consumes: runtime-only `SITE_ADDRESS`, confirmed administrator email, and `aliuapi.top` allowlist host.
- Produces: a validated mode-600 production environment and a four-service `sub2api` Compose project.

- [x] **Step 1: Upload only controlled deployment inputs**

Create `/opt/sub2api/production` owned by `ubuntu`, then copy `infra/compose.yaml`, `infra/Caddyfile`, `infra/.env.example`, and `ops/generate-env.sh`. Do not copy local `infra/.env`.

- [x] **Step 2: Generate secrets on the server**

Run the uploaded generator once to create `/opt/sub2api/production/.env`. Require mode `600`, `ubuntu:ubuntu` ownership, and five distinct 64-character lowercase hexadecimal secrets. Validate only shapes and equality; do not print values.

- [x] **Step 3: Set non-secret production values without printing the environment**

Replace exactly `SITE_ADDRESS`, `ADMIN_EMAIL`, and `SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS` in the server-side `.env`. Require the allowlist value to equal `aliuapi.top`. Do not display the file.

- [x] **Step 4: Validate images and Compose before changing the running service**

Run `docker compose --env-file .env -f compose.yaml config --quiet`, validate the Caddyfile with the pinned Caddy image, pull all pinned images, and require the Sub2API image version to contain `0.1.155`.

- [x] **Step 5: Atomically replace bootstrap with the full stack**

Stop the bootstrap Compose service without `-v`, then run `docker compose --project-name sub2api --env-file .env -f compose.yaml up -d --wait` from the production directory.

Rollback on failure: run full-stack `down` without `-v`, then restart `/opt/sub2api/bootstrap/compose.bootstrap.yaml` with the runtime-only `SITE_ADDRESS`. Confirm the bootstrap `/health` endpoint before stopping.

### Task 3: Production acceptance and durable handoff

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Modify: `docs/superpowers/plans/2026-07-16-production-compose-first-deployment.md`

**Interfaces:**
- Consumes: the running full stack and operator-only HTTPS endpoint.
- Produces: verified M1 service state and a safe next step for entering the replacement upstream Key through the Sub2API management UI.

- [x] **Step 1: Verify container health and port isolation**

Require `postgres`, `redis`, and `sub2api` to be healthy, `caddy` to be running, and no bootstrap container to remain. Require only 80/443 to be published; 5432, 6379, and 8080 must not listen on the host.

- [x] **Step 2: Verify HTTPS application health and certificate**

Using `curl --resolve`, require HTTP 308, HTTPS `/health` 200, and a publicly trusted certificate whose SAN matches the runtime-only hostname.

- [x] **Step 3: Verify administrator bootstrap without exposing the password**

Query PostgreSQL inside the Compose network and require exactly one administrator record matching the confirmed email. Do not print password hashes or secrets.

- [x] **Step 4: Verify persistence across a controlled restart**

Restart only the `sub2api` service, wait for healthy status, and confirm the administrator count remains one and HTTPS `/health` remains 200.

- [x] **Step 5: Update non-sensitive project state**

Record full-stack health, port isolation, administrator initialization, and the remaining upstream-Key/UI step without recording the hostname, instance IP, administrator email, generated password, or any other credential.

- [x] **Step 6: Run final verification**

Re-run the infrastructure contract, live HTTPS and TLS checks, remote container and port checks, and a controlled-file credential scan. All commands must exit 0.

## Acceptance

- [x] The four-service stack is healthy and survives a controlled Sub2API restart.
- [x] Only Caddy publishes 80/443; internal service ports are not exposed.
- [x] HTTPS uses the existing publicly trusted temporary certificate and `/health` returns 200.
- [x] Exactly one administrator exists for the confirmed email, without exposing its generated password.
- [x] No upstream API Key has been used or stored; replacement-Key entry remains a user action in the management UI.
- [x] Rollback remains available without deleting named volumes.

## Self-Review

- The plan changes one production subsystem and includes a preflight, explicit rollback, and post-switch acceptance.
- Runtime addresses and credentials are excluded from durable artifacts.
- It does not depend on a permanent domain or an upstream Key to establish M1 core-site health.
- It preserves the Caddy certificate cache and all application data volumes.
