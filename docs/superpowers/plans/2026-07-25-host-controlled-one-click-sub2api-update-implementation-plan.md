# Host-Controlled One-Click Sub2API Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve the official Sub2API update notification and button while moving immediate and one-time scheduled Docker upgrades into a durable host service that backs up, recreates only `sub2api`, validates, and rolls back on failure.

**Architecture:** Caddy injects a small same-origin UI script into the official SPA HTML and intercepts only Docker-mutating system endpoints. A root-owned Go systemd service authenticates the existing admin bearer against official Sub2API, persists at most one immediate/scheduled operation, resolves the approved official Docker digest, and invokes a host-only named-volume release script. Official Sub2API source and image remain unmodified.

**Tech Stack:** Go 1.24 standard library, POSIX/Bash, Docker Compose v2, Caddy templates and Unix-socket reverse proxy, systemd, vanilla browser JavaScript, Ruby/Node-free shell fixtures.

## Global Constraints

- Production commands run only through `ssh sub2api-prod`, Linux Docker context `default`, directory `/opt/sub2api/production`, Compose project `sub2api`.
- The runtime Sub2API image must always be `weishaw/sub2api:<version>@sha256:<digest>`.
- Never invoke Sub2API's in-container update, rollback, or restart implementation.
- Never call `docker compose down`; recreate only `sub2api` with `--no-deps --force-recreate`.
- PostgreSQL, Redis, Caddy, relay-ops, and D04 container IDs must not change during an application update.
- Production named volumes remain `sub2api_sub2api_data`, `sub2api_postgres_data`, and `sub2api_redis_data`.
- Every mutation requires an active human admin Bearer session, exact production Origin, and `X-Admin-UI-Request: 1`; admin API keys are not accepted.
- Persist one operation at most. Store timestamps in UTC RFC3339; accept/display schedule input in `Asia/Shanghai`; schedule bounds are 2 minutes through 30 days.
- A scheduled operation is pinned to the version and Docker digest resolved when it is created; it never floats to a later release.
- Backups and state are `0700/0600`, contain no credentials, and are retained after success or rollback.
- Failure after mutation restores the previous image and recreates only `sub2api`; database restore is never automatic.
- Keep `.worktrees/xingqiao-beginner-guide` untouched.

---

### Task 1: Durable updater service, authentication, and one-time scheduling

**Files:**
- Create: `sub2api-updater/go.mod`
- Create: `sub2api-updater/cmd/sub2api-updater/main.go`
- Create: `sub2api-updater/internal/updater/service.go`
- Create: `sub2api-updater/internal/updater/http.go`
- Create: `sub2api-updater/internal/updater/store.go`
- Create: `sub2api-updater/internal/updater/resolver.go`
- Create: `sub2api-updater/internal/updater/auth.go`
- Test: `sub2api-updater/internal/updater/*_test.go`

**Interfaces:**
- Consumes: official GitHub latest release, `docker pull`/`docker image inspect`, Sub2API `/api/v1/auth/me`, executor path from Task 2.
- Produces:
  - `POST /api/v1/admin/system/update` body `{"mode":"now|schedule","target_version":"x.y.z","scheduled_at":"RFC3339"}`.
  - `GET /api/v1/admin/system/host-update/status`.
  - `DELETE /api/v1/admin/system/host-update/schedule`.
  - State schema fields `schema_version`, `operation_id`, `actor_id`, `mode`, `target_version`, `image`, `scheduled_at`, `created_at`, `started_at`, `completed_at`, `stage`, `result`, `error`.

- [ ] **Step 1: Write failing tests for request admission**

Create table-driven tests proving that mutations reject missing/malformed Bearer, wrong Origin, missing `X-Admin-UI-Request`, non-JSON bodies, cross-site fetch metadata, inactive/non-admin identity, empty mode, target different from GitHub latest, and schedule outside `[now+2m, now+30d]`. Prove status requires the same active admin session and that no rejected request calls the resolver or executor.

- [ ] **Step 2: Run the admission tests and confirm RED**

Run: `cd sub2api-updater && go test ./internal/updater -run 'TestHTTP.*Reject|TestHTTP.*Auth' -count=1`

Expected: FAIL because the package and handlers do not exist.

- [ ] **Step 3: Implement the authentication and HTTP envelope**

Use standard `net/http`. Parse exactly one `Bearer` token, validate headers, then verify `/api/v1/auth/me` by forwarding Bearer, User-Agent, X-Forwarded-For, and X-Real-IP. Accept only positive ID, role `admin`, status `active`. Responses use the official shape:

```json
{"code":0,"data":{"operation_id":"...","stage":"scheduled"}}
```

Errors use stable codes: `UPDATE_CONFIRMATION_REQUIRED`, `UPDATE_AUTH_REQUIRED`, `UPDATE_FORBIDDEN`, `UPDATE_ALREADY_SCHEDULED`, `UPDATE_IN_PROGRESS`, `UPDATE_INVALID_TIME`, `UPDATE_TARGET_CHANGED`, and `UPDATE_SERVICE_ERROR`.

- [ ] **Step 4: Write failing store/scheduler tests**

Cover atomic `0600` JSON replacement, corrupt state fail-closed, same-operation idempotency, one scheduled/running operation, replacement only after DELETE, timer firing once, cancellation before start, cancellation losing the race to a running job, restart recovery, overdue execution on startup, and persisted immutable image ref.

- [ ] **Step 5: Run store/scheduler tests and confirm RED**

Run: `cd sub2api-updater && go test ./internal/updater -run 'Test(Store|Service|Scheduler)' -count=1`

Expected: FAIL for missing store and service behavior.

- [ ] **Step 6: Implement state, resolver, and scheduler**

Use a mutex plus atomic temp-file/rename storage. `Resolver.Resolve(ctx, targetVersion)` fetches GitHub latest, requires exact normalized version equality, runs `docker pull weishaw/sub2api:<version>`, reads the matching `RepoDigest`, and returns `weishaw/sub2api:<version>@sha256:<64 hex>`. `Service` starts one timer, reloads state on startup, calls `Executor.Run(ctx, operation)` once, and maps executor output to `promoted`, `rolled_back`, `failed`, or `rollback_failed`.

- [ ] **Step 7: Run service tests**

Run: `cd sub2api-updater && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

```bash
git add sub2api-updater
git commit -m "feat: add durable Sub2API update scheduler"
```

---

### Task 2: Host named-volume update executor

**Files:**
- Create: `ops/update-sub2api-host.sh`
- Create: `tests/operations/update_sub2api_host_test.sh`
- Modify: `docs/runbooks/sub2api-official-image-release.md`

**Interfaces:**
- Consumes: `--image weishaw/sub2api:<version>@sha256:<digest>` and `--operation-id <id>` from Task 1.
- Produces stdout exactly one terminal line: `result=promoted`, `result=rolled_back`, or `result=rollback_failed`; writes a `0600` release record under `/opt/sub2api/production/release-records/host-updater/`.

- [ ] **Step 1: Write the failing shell fixture**

Use fake `docker`, `curl`, `sha256sum`, `tar`, `jq`, and time commands. Assert rejection before Docker mutation for Darwin, non-default context, wrong directory/project, mutable or wrong-repository image ref, unexpected volume names, unhealthy dependencies, insufficient space, or an existing lock. Assert the success trace is:

```text
inspect -> pull -> backup-db -> backup-counts -> backup-app-data -> checksum -> compose-validate -> recreate-sub2api -> health -> smoke -> promoted
```

Assert PostgreSQL/Redis/Caddy/relay/D04 IDs remain identical and no `down`, dependency recreate, or database restore command appears.

- [ ] **Step 2: Run the fixture and confirm RED**

Run: `bash tests/operations/update_sub2api_host_test.sh`

Expected: FAIL because the executor is absent.

- [ ] **Step 3: Implement preflight and verified backup**

The script uses `set -euo pipefail`, `umask 077`, a `mkdir` lock, explicit project/directory/env/Compose arguments, exact volume/source validation, at least 2 GiB free, a PostgreSQL custom dump validated by pinned PostgreSQL 18, six record counts, `/app/data` tar, Compose and container ID snapshots, `SHA256SUMS`, and modes `0700/0600`.

- [ ] **Step 4: Implement exact-image recreation and rollback**

Tag the previous image ID, update only the first `services.sub2api.image` declaration through an atomic file replacement, validate resolved Compose JSON, run `up -d --no-deps --force-recreate sub2api`, wait at most 180 seconds, and validate exact image ID/digest and unchanged volume. Smoke checks cover `/health`, counts monotonicity, exactly one `xingqiao-support` `md:support`, QR hash `35b84b14ab472e117fa413ed5f91357becd01199eeaf3fed469a2d9d3d987c16`, and zero recent fatal/migration errors. On failure restore the old image declaration and recreate only Sub2API.

- [ ] **Step 5: Run executor tests and lint**

Run:

```bash
bash tests/operations/update_sub2api_host_test.sh
shellcheck ops/update-sub2api-host.sh tests/operations/update_sub2api_host_test.sh
```

Expected: PASS.

- [ ] **Step 6: Document the one-command and web paths**

Document that CLI invocation and the web button call the same executor; document status, cancellation, backup, rollback tag, and incident paths without including secret values.

- [ ] **Step 7: Commit Task 2**

```bash
git add ops/update-sub2api-host.sh tests/operations/update_sub2api_host_test.sh docs/runbooks/sub2api-official-image-release.md
git commit -m "feat: add named-volume Sub2API update executor"
```

---

### Task 3: Official-button confirmation UI and Caddy routing

**Files:**
- Create: `infra/sub2api-update-ui/index.html`
- Create: `infra/sub2api-update-ui/update-ui.js`
- Create: `infra/sub2api-update-ui/update-ui.css`
- Create: `tests/infra/sub2api-update-ui.test.mjs`
- Create: `tests/infra/validate-sub2api-update-routing.sh`
- Modify: `infra/Caddyfile`
- Modify: `infra/compose.yaml`

**Interfaces:**
- Consumes: Task 1 API and official `/admin/system/check-updates` response.
- Produces: capture-phase interception of the official localized `立即更新`/`Update Now` button, accessible modal, immediate/scheduled/cancel actions, and polling status view.

- [ ] **Step 1: Write failing DOM tests**

Using the workspace Node runtime and a minimal DOM fixture, prove the injected script:
+
+- intercepts the official button before its Vue handler;
+- opens a dialog showing current and target versions;
+- defaults to `现在升级`;
+- converts a `datetime-local` value as `Asia/Shanghai` to UTC RFC3339;
+- requires explicit confirmation;
+- shows/replaces/cancels one existing schedule;
+- polls after accepted/running operations;
+- does not intercept unrelated buttons;
+- fail-closes by intercepting the direct mutation endpoint even if the selector no longer matches.

- [ ] **Step 2: Run DOM tests and confirm RED**

Run: `node --test tests/infra/sub2api-update-ui.test.mjs`

Expected: FAIL because the UI assets do not exist.

- [ ] **Step 3: Implement version-independent UI assets**

Use no framework and no inline secret/config. Read the existing `auth_token` from localStorage only at request time; never log it. Use `role=dialog`, focus trapping, Escape/取消 behavior, visible Beijing time, minimum/maximum input, disabled submit while pending, and sanitized text nodes. POST `mode=now` or `mode=schedule` with the checked target version.

- [ ] **Step 4: Write failing Caddy/Compose contract tests**

Require:
+
+- GET `/api/v1/admin/system/check-updates` still reaches Sub2API;
+- POST update and host-update status/schedule routes reach `unix//run/sub2api-updater/updater.sock`;
+- POST rollback/restart never reach Sub2API;
+- HTML navigation is templated exactly once with official `httpInclude` plus the external JS/CSS;
+- API/static requests are not HTML-templated;
+- Caddy has read-only binds for `./sub2api-update-ui` and `/run/sub2api-updater`.

- [ ] **Step 5: Run routing tests and confirm RED**

Run: `bash tests/infra/validate-sub2api-update-routing.sh`

Expected: FAIL on absent routes/mounts.

- [ ] **Step 6: Implement Caddy template and Unix-socket routes**

Add a private internal upstream route for the official index, serve the template and immutable UI assets, intercept update/status/cancel before generic proxy, set update response timeout to 15 minutes, and preserve existing homepage/support/D04/relay routes and CSP. Add only the two read-only Caddy mounts; do not mount Docker socket into any container.

- [ ] **Step 7: Run UI and routing tests**

Run:
+
```bash
node --test tests/infra/sub2api-update-ui.test.mjs
bash tests/infra/validate-sub2api-update-routing.sh
```
+
Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```bash
git add infra/sub2api-update-ui infra/Caddyfile infra/compose.yaml tests/infra
git commit -m "feat: bind official update button to host updater"
```

---

### Task 4: systemd packaging and production deployment

**Files:**
- Create: `infra/systemd/sub2api-updater.service`
- Create: `infra/systemd/sub2api-updater.env.example`
- Create: `ops/install-sub2api-updater.sh`
- Create: `tests/operations/install_sub2api_updater_test.sh`
- Modify: `docs/runbooks/sub2api-official-image-release.md`
- Create: `docs/superpowers/reports/2026-07-25-host-controlled-one-click-sub2api-update-verification.md`

**Interfaces:**
- Consumes: Linux amd64 updater binary, executor, UI assets, Caddy/Compose declarations.
- Produces: root-owned service, Unix socket group-readable by Caddy's container bind, durable state directory, and repeatable install/rollback commands.

- [ ] **Step 1: Write failing packaging tests**

Assert service hardening (`User=root`, dedicated runtime/state dirs, `UMask=0077`, `NoNewPrivileges=true`, restricted write paths, restart policy), exact environment paths, binary/script ownership and modes, `systemd-analyze verify`, and installer refusal off the production host boundary.

- [ ] **Step 2: Run packaging tests and confirm RED**

Run: `bash tests/operations/install_sub2api_updater_test.sh`

Expected: FAIL because package files are absent.

- [ ] **Step 3: Implement units and installer**

Cross-compile with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`; install binary `0755`, executor `0700`, env `0600`, state dir `0700`, runtime socket directory with the numeric group used by Caddy, unit `0644`, then daemon-reload and enable/start. Installer validates files before changing systemd and never prints env values.

- [ ] **Step 4: Run all focused tests**

Run:
+
```bash
(cd sub2api-updater && go test ./... -count=1)
bash tests/operations/update_sub2api_host_test.sh
node --test tests/infra/sub2api-update-ui.test.mjs
bash tests/infra/validate-sub2api-update-routing.sh
bash tests/operations/install_sub2api_updater_test.sh
```
+
Expected: PASS.

- [ ] **Step 5: Deploy without triggering an application update**

Over `ssh sub2api-prod`: capture all container IDs and counts; install updater; back up Compose/Caddy; copy UI assets; validate systemd and Caddy; update only Caddy mounts/routes; recreate only Caddy if required to pick up new mounts. Do not submit `mode=now` against a newer version during deployment.

- [ ] **Step 6: Production acceptance**

Authenticate through the existing browser session. Prove official version check still reports current/latest; modal opens; create a one-time future schedule for the current already-up-to-date target only if the service treats it as a no-op, then cancel it; otherwise use the authenticated status endpoint and unit tests without creating a task. Verify socket permissions, state mode, service health, public `/health`, all original container IDs except an explicitly recreated Caddy, PostgreSQL/Redis IDs unchanged, counts unchanged, update/rollback direct endpoint guard, and zero secret output.

- [ ] **Step 7: Write the production report**

Record exact image/service identities, commands by purpose, checks, backup/rollback paths, UI acceptance, schedule/cancel result, and residual risk. Do not include session tokens, env contents, admin key, database dump content, or full private identifiers.

- [ ] **Step 8: Commit Task 4**

```bash
git add infra/systemd ops/install-sub2api-updater.sh tests/operations/install_sub2api_updater_test.sh docs
git commit -m "ops: deploy host-controlled Sub2API updates"
```

---

### Task 5: Whole-workspace verification, main integration, push, and archive

**Files:**
- Modify as required by failing tests only.
- Update: `.superpowers/sdd/progress.md` (local ledger; ignored if configured).

**Interfaces:**
- Consumes: Tasks 1-4 plus all pre-existing dirty workspace work.
- Produces: verified `origin/main`, local branch `main`, deleted merged feature branch, preserved independent worktree, archived Codex task.

- [ ] **Step 1: Run the complete available verification matrix**

Run at minimum:
+
```bash
git diff --check
(cd homepage && npm test -- --run && npm run build)
(cd relay-ops-service && go test ./... -count=1)
(cd sub2api-updater && go test ./... -count=1)
bash tests/infra/validate-baseline.sh
bash tests/infra/validate-official-sub2api-release.sh
bash tests/infra/validate-sub2api-update-routing.sh
bash tests/operations/configure_sub2api_support_test.sh
bash tests/operations/backup_sub2api_release_test.sh
bash tests/operations/deploy_sub2api_release_test.sh
bash tests/operations/smoke_sub2api_release_test.sh
bash tests/operations/update_sub2api_host_test.sh
bash tests/operations/install_sub2api_updater_test.sh
bash tests/relay_ops/validate_relay_ops_contract.sh
```
+
Fix only evidenced failures and rerun the covering and full matrices.

- [ ] **Step 2: Secret and payload review**

Review `git status`, all staged paths, large files, executable modes, and high-confidence secret patterns. Confirm the intentionally committed `upstream/sub2api` is a source snapshot without `.git`, build output, node_modules, secrets, or private environment files.

- [ ] **Step 3: Commit all non-ignored workspace content**

```bash
git add -A
git diff --cached --check
git commit -m "feat: complete official Sub2API operations workspace"
```

- [ ] **Step 4: Fetch and integrate main**

```bash
git fetch origin main
git merge-base --is-ancestor origin/main codex/l1-2-offline-baseline
git switch -c main --track origin/main
git merge --ff-only codex/l1-2-offline-baseline
```

If remote main advanced and is not an ancestor, merge the feature branch into current `origin/main` without force-pushing, resolve conflicts, and rerun the complete verification matrix.

- [ ] **Step 5: Verify and push main**

Rerun the complete matrix on `main`, then:
+
```bash
git push origin main:main
test "$(git rev-parse main)" = "$(git ls-remote origin refs/heads/main | awk '{print $1}')"
```

- [ ] **Step 6: Archive the workspace safely**

Delete the merged `codex/l1-2-offline-baseline` branch. Do not remove the main repository directory. Leave `.worktrees/xingqiao-beginner-guide` unchanged. Archive the current Codex task using the app task-archive capability after all final evidence is reported.
