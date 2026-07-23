# Temporary sslip.io HTTPS Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a zero-cost, temporary, publicly trusted HTTPS health endpoint before production secrets and the permanent domain are available.

**Architecture:** Run only the pinned Caddy image on the production host with a bootstrap Caddyfile. The temporary `sslip.io` hostname is derived from the instance IPv4 at runtime and is never persisted in project documentation; the bootstrap Compose project reuses the final `sub2api_caddy_data` and `sub2api_caddy_config` volumes so the certificate cache survives the later transition.

**Tech Stack:** Docker Compose v5, Caddy 2.10.2, sslip.io, Let's Encrypt/ZeroSSL through Caddy Automatic HTTPS, Bash, curl, OpenSSL.

## Global Constraints

- The temporary hostname is for operator-only technical validation, not external testers or customer traffic.
- Do not purchase a domain, add an item to a cart, or change DNS records.
- Do not read or output `infra/.env`.
- Do not persist the server IPv4, SSH private key, generated hostname, or credentials in Git or project documentation.
- Do not start, stop, or recreate the existing local `sub2api`, `sub2api-postgres`, or `sub2api-redis` containers.
- Only ports 80 and 443 may be published by the bootstrap container.

---

### Task 1: Reproducible bootstrap configuration

**Files:**
- Create: `infra/compose.bootstrap.yaml`
- Create: `infra/Caddyfile.bootstrap`
- Modify: `tests/infra/validate-baseline.sh`

**Interfaces:**
- Consumes: runtime-only `SITE_ADDRESS` environment variable.
- Produces: a pinned Caddy-only Compose service exposing `/health` and persistent Caddy certificate volumes.

- [x] **Step 1: Extend the infrastructure contract test**

Require both bootstrap files, parse the bootstrap Compose configuration with `SITE_ADDRESS=api.example.com`, assert the pinned Caddy digest, assert that only 80/443 are published, and assert the exact bootstrap health response marker.

- [x] **Step 2: Run the contract test and verify it fails**

Run: `tests/infra/validate-baseline.sh`

Expected: `FAIL: missing infra/compose.bootstrap.yaml`.

- [x] **Step 3: Add the bootstrap Compose file**

Create a `sub2api` Compose project with one `caddy-bootstrap` service, the existing pinned Caddy digest, `SITE_ADDRESS: ${SITE_ADDRESS:?required}`, ports 80/443, read-only bootstrap Caddyfile mount, and named `caddy_data`/`caddy_config` volumes.

- [x] **Step 4: Add the bootstrap Caddyfile**

Serve `{"status":"ok","phase":"bootstrap"}` with HTTP 200 only at `/health`, return HTTP 404 elsewhere, remove the `Server` header, and emit JSON access logs.

- [x] **Step 5: Run the contract test and verify it passes**

Run: `tests/infra/validate-baseline.sh`

Expected: exit 0 with the existing baseline success message.

### Task 2: Deploy temporary HTTPS bootstrap

**Files:**
- Remote create: `/opt/sub2api/bootstrap/compose.bootstrap.yaml`
- Remote create: `/opt/sub2api/bootstrap/Caddyfile.bootstrap`

**Interfaces:**
- Consumes: the runtime-derived `SITE_ADDRESS` and the dedicated SSH identity.
- Produces: `sub2api-caddy-bootstrap-1` plus persistent `sub2api_caddy_data` and `sub2api_caddy_config` volumes.

- [x] **Step 1: Verify authoritative resolution from the host**

Run `getent ahostsv4 "$SITE_ADDRESS"` on the server and require the first address to equal the instance IPv4 held only in the current shell session.

- [x] **Step 2: Copy the two bootstrap files**

Create `/opt/sub2api/bootstrap` owned by `ubuntu`, then copy only `infra/compose.bootstrap.yaml` and `infra/Caddyfile.bootstrap` to that directory.

- [x] **Step 3: Validate the remote Caddy and Compose configurations**

Run the pinned Caddy image with `caddy validate --config /etc/caddy/Caddyfile`, then run `docker compose --project-name sub2api --env-file /dev/null -f compose.bootstrap.yaml config --quiet` with `SITE_ADDRESS` exported.

- [x] **Step 4: Start the bootstrap service**

Run `docker compose --project-name sub2api -f compose.bootstrap.yaml up -d --wait` with `SITE_ADDRESS` exported.

Expected: only `sub2api-caddy-bootstrap-1` is created; no database, Redis, or Sub2API service is started.

### Task 3: End-to-end HTTPS acceptance and handoff

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes: the live bootstrap endpoint.
- Produces: verified TLS evidence and an explicit gate for replacing the temporary hostname with the permanent domain.

- [x] **Step 1: Verify HTTP redirect and HTTPS health**

Use `curl --resolve` from the current Mac so local Fake-IP DNS cannot affect the test. Require HTTP to redirect to HTTPS and HTTPS `/health` to return the exact bootstrap JSON with status 200.

- [x] **Step 2: Verify certificate identity and expiry**

Use `openssl s_client -servername "$SITE_ADDRESS"` and `openssl x509 -noout -subject -issuer -dates -ext subjectAltName`; require the temporary hostname in SAN and a currently valid public issuer.

- [x] **Step 3: Verify port and container isolation**

On the host, require one running bootstrap container, no other Docker containers, and listeners only on SSH/DNS plus Caddy 80/443. Confirm 5432, 6379, and 8080 are not published.

- [x] **Step 4: Update non-sensitive project state**

Record that temporary HTTPS bootstrap passed without recording the generated hostname or instance IP. Set the next gate to administrator email, upstream hostname, and a secure production-secret workflow before the full Compose deployment.

- [x] **Step 5: Final verification**

Re-run `tests/infra/validate-baseline.sh`, the HTTPS health request, the TLS certificate inspection, and the project-document secret scan. All commands must exit 0.

## Self-Review

- Scope is limited to Caddy and temporary HTTPS; production application secrets and upstream calls are excluded.
- The runtime hostname and IPv4 are explicitly excluded from controlled files.
- Bootstrap and final Compose share only Caddy certificate volumes; no application data volume is created early.
- The repository has no existing commit history and the entire worktree is currently untracked, so this plan does not create an implicit initial commit.
