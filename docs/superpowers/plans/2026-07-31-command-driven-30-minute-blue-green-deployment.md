# Command-Driven 30-Minute Blue-Green Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a production-safe command-driven release path that builds the already-tested Git tree into an immutable image and performs a 30-minute blue-green Sub2API deployment, while stopping before any production mutation whenever downtime is required.

**Architecture:** Run two permanent API-only slots (`sub2api-blue`, `sub2api-green`) behind a Caddy environment-selected upstream and one unique `sub2api-worker` process for migrations and shared singleton jobs. A local release controller proves tested-tree identity and builds/publishes an immutable digest; a host executor performs fail-closed preflight, starts only the inactive API slot, gracefully reloads Caddy, updates the worker after public acceptance, and automatically cuts back to the previous slot on post-cutover failure.

**Tech Stack:** Go 1.24, Google Wire, PostgreSQL 18, Redis 8, Docker Compose v2, Caddy 2, Bash 3.2-compatible shell, Ruby JSON helpers, jq, curl, Docker Buildx, existing Sub2API release/candidate transport.

## Global Constraints

- No production connection or mutation occurs until the user explicitly issues a production deployment command.
- A normal production deployment command never implies downtime authorization.
- The end-to-end production command has a hard 1800-second budget.
- Deploy only an immutable `sha256` image digest whose source tree exactly matches fresh test evidence.
- `sub2api-blue` and `sub2api-green` always run with `SERVER_PROCESS_ROLE=api`.
- Exactly one `sub2api-worker` runs with `SERVER_PROCESS_ROLE=worker`.
- API roles perform no migration DDL/INSERT, secret bootstrap, simple-mode seed, startup Setting migration, scheduler, monitor, or shared queue consumption.
- Request-local background work required by the serving request path remains enabled in API roles.
- Caddy, PostgreSQL, Redis, volumes, Docker network, and Compose project identity are never rebuilt by a normal blue-green release.
- Never run `docker compose down`, delete production volumes, or automatically restore PostgreSQL.
- Caddy upstream values are restricted to `sub2api-blue:8080` and `sub2api-green:8080`.
- Any migration-set change, topology bootstrap uncertainty, insufficient parallel capacity, incompatible shared state, or unprovable rollback returns `downtime_required=true` before production mutation.
- First migration from legacy single-service topology is a separate bootstrap gate and may not claim the steady-state zero-downtime guarantee.
- All behavior changes use test-first red-green-refactor cycles; generated Wire output is regenerated only after its source wiring tests pass.
- Each implementation task is assigned to a fresh implementer subagent, independently reviewed, and followed by a final whole-branch review.
- This implementation ends in `准备完成/待生产验收`; it is not `已完成` until pushed, deployed, and verified on production.

---

## File Map

**Create**

- `upstream/sub2api/backend/internal/repository/migrations_verify_test.go` — read-only migration verification tests.
- `upstream/sub2api/backend/internal/service/process_role_test.go` — request-local versus singleton startup policy tests.
- `upstream/sub2api/backend/internal/securityaudit/prompt_service_role_test.go` — config-only versus queue-consumer lifecycle tests.
- `tests/operations/sub2api_blue_green_topology_test.sh` — rendered Compose/Caddy behavior contract.
- `ops/deploy-sub2api-blue-green-host.sh` — production-host blue-green executor.
- `tests/operations/deploy_sub2api_blue_green_host_test.sh` — fake Docker/Caddy/curl host executor behavior tests.
- `ops/write-sub2api-test-evidence.sh` — atomic tested-tree evidence writer.
- `ops/release-sub2api-blue-green.sh` — local build/publish/orchestration entrypoint.
- `tests/operations/release_sub2api_blue_green_test.sh` — release controller behavior tests.
- `docs/runbooks/sub2api-blue-green-production-deployment.md` — operator contract, bootstrap gate, recovery, and future production command.
- `docs/superpowers/reports/2026-07-31-command-driven-blue-green-local-verification.md` — local verification evidence and unresolved production gates.

**Modify**

- `upstream/sub2api/backend/internal/config/config.go` — process-role type, parsing, default, and validation.
- `upstream/sub2api/backend/internal/config/config_test.go` — default/valid/invalid process-role tests.
- `upstream/sub2api/backend/internal/repository/ent.go` — role-aware migrate/verify/bootstrap/seed startup.
- `upstream/sub2api/backend/internal/repository/migrations_runner.go` — zero-write migration verification and migration-set hash.
- `upstream/sub2api/backend/internal/service/wire.go` — classify and gate singleton startup while preserving request-local workers.
- `upstream/sub2api/backend/internal/service/wire_test.go` — provider-level role behavior where existing concrete services permit direct observation.
- `upstream/sub2api/backend/internal/securityaudit/prompt_service.go` — separate config lifecycle from shared queue consumption.
- `upstream/sub2api/backend/cmd/server/main.go` — start Prompt Audit using the configured process role.
- `upstream/sub2api/backend/cmd/server/wire_gen.go` — regenerated Wire graph.
- `infra/compose.yaml` — blue, green, and worker topology with shared environment anchors.
- `infra/Caddyfile` — allowlisted environment-selected Sub2API upstream and internal active route.
- `infra/sub2api-candidate-loader.env.example` — release state/evidence/host executor variables.
- `.github/workflows/sub2api-release-preparation.yml` — write source tree, tested tree, and migration-set labels.
- `infra/compose.sub2api-rehearsal.yaml` — isolated steady-state blue-green rehearsal.
- `docs/project/current-state.md` — current deployment mechanism truth.
- `docs/project/project-progress.md` — keep the initiative in progress, then move it only to ready-for-production-validation.

---

### Task 1: Add Process Roles and Read-Only Migration Verification

**Files:**

- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/config/config_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/ent.go`
- Modify: `upstream/sub2api/backend/internal/repository/migrations_runner.go`
- Create: `upstream/sub2api/backend/internal/repository/migrations_verify_test.go`

**Interfaces:**

- Produces:

```go
type ProcessRole string

const (
    ProcessRoleAll    ProcessRole = "all"
    ProcessRoleAPI    ProcessRole = "api"
    ProcessRoleWorker ProcessRole = "worker"
)

func ParseProcessRole(string) (ProcessRole, error)
func (r ProcessRole) ServesAPI() bool
func (r ProcessRole) RunsMigrations() bool
func (r ProcessRole) RunsSingletonJobs() bool

type ServerConfig struct {
    // existing fields remain unchanged
    ProcessRole ProcessRole `mapstructure:"process_role"`
}

func VerifyMigrations(ctx context.Context, db *sql.DB) error
func MigrationSetHash(fsys fs.FS) (string, error)
```

- `SERVER_PROCESS_ROLE` maps through Viper to `server.process_role`; the default is `all`.
- `MigrationSetHash` is SHA-256 over the sorted sequence `filename + "\x00" + normalized-file-checksum + "\n"` for each non-empty `*.sql` migration.
- `VerifyMigrations` reads only `schema_migrations`; it neither creates the table nor inserts/updates rows.

- [ ] **Step 1: Write failing process-role config tests**

Add table-driven tests proving `Load()` defaults to `all`, accepts case/space-normalized `api` and `worker`, and rejects `SERVER_PROCESS_ROLE=primary`, empty-after-normalization supplied explicitly, and other unknown values with an error containing `server.process_role`.

- [ ] **Step 2: Run the focused config tests and verify RED**

```bash
go -C upstream/sub2api/backend test ./internal/config -run 'ProcessRole' -count=1
```

Expected: compile failure because `ProcessRole` and `ServerConfig.ProcessRole` do not exist.

- [ ] **Step 3: Implement process-role parsing and validation**

Set the Viper default `server.process_role=all`, normalize through `ParseProcessRole`, and expose the three role capability methods. Preserve `all` as backward-compatible behavior.

- [ ] **Step 4: Run config tests and verify GREEN**

```bash
go -C upstream/sub2api/backend test ./internal/config -run 'ProcessRole' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing zero-write migration verification tests**

Use `go-sqlmock` with a small `fstest.MapFS` containing two migrations. Cover:

```go
// both filename/checksum rows exist => nil
// missing schema_migrations table => descriptive error, no Exec expectations
// missing migration row => error naming the filename, no Exec expectations
// checksum mismatch => error naming db and file checksum, no Exec expectations
// known compatibility-rule checksum => accepted
// MigrationSetHash is stable across MapFS insertion order and changes with file content
```

At the end of every case call `mock.ExpectationsWereMet()`; defining no `ExpectExec` is the proof that the API path performs zero writes.

- [ ] **Step 6: Run repository tests and verify RED**

```bash
go -C upstream/sub2api/backend test ./internal/repository -run 'VerifyMigrations|MigrationSetHash' -count=1
```

Expected: compile failure because the verification functions do not exist.

- [ ] **Step 7: Implement read-only verification and role-aware `InitEnt`**

Refactor migration file enumeration/checksum calculation into shared pure helpers. In `InitEnt`:

```go
if cfg.Server.ProcessRole.RunsMigrations() {
    err = applyMigrationsFS(migrationCtx, drv.DB(), migrations.FS)
} else {
    err = verifyMigrationsFS(migrationCtx, drv.DB(), migrations.FS)
}
```

Run `ensureBootstrapSecrets`, post-bootstrap validation, and simple-mode seed only when `RunsMigrations()` is true. API role still runs `cfg.Validate()` so missing explicit secrets fail closed.

- [ ] **Step 8: Run focused and package tests**

```bash
go -C upstream/sub2api/backend test ./internal/config ./internal/repository -run 'ProcessRole|VerifyMigrations|MigrationSetHash|MigrationChecksum' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 1**

```bash
git add upstream/sub2api/backend/internal/config/config.go \
  upstream/sub2api/backend/internal/config/config_test.go \
  upstream/sub2api/backend/internal/repository/ent.go \
  upstream/sub2api/backend/internal/repository/migrations_runner.go \
  upstream/sub2api/backend/internal/repository/migrations_verify_test.go
git commit -m "feat: add safe Sub2API process roles"
```

---

### Task 2: Separate Request-Local and Singleton Background Lifecycles

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/service/wire_test.go`
- Create: `upstream/sub2api/backend/internal/service/process_role_test.go`
- Modify: `upstream/sub2api/backend/internal/securityaudit/prompt_service.go`
- Create: `upstream/sub2api/backend/internal/securityaudit/prompt_service_role_test.go`
- Modify: `upstream/sub2api/backend/cmd/server/main.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**

- Produces:

```go
func ShouldStartRequestLocal(role config.ProcessRole) bool
func ShouldStartSingleton(role config.ProcessRole) bool

type PromptStartMode struct {
    ConsumeSharedQueue bool
}

func (s *PromptService) Start(ctx context.Context, mode PromptStartMode) error
```

- `ShouldStartRequestLocal` is true for `all` and `api`, false for `worker`.
- `ShouldStartSingleton` is true for `all` and `worker`, false for `api`.
- Prompt config synchronization starts for every role; its shared `Runner` starts only when `ConsumeSharedQueue=true`.

- [ ] **Step 1: Write failing lifecycle policy tests**

Add a six-row table covering both policy functions for `all`, `api`, and `worker`. Name the production mutation each row protects: enabling a singleton in API or disabling request-local flush in API must fail the tests.

- [ ] **Step 2: Run the lifecycle test and verify RED**

```bash
go -C upstream/sub2api/backend test ./internal/service -run 'ProcessRoleLifecycle' -count=1
```

Expected: compile failure because the policy functions do not exist.

- [ ] **Step 3: Implement the policy and gate every provider start**

Pass `*config.Config` into providers that start background work and classify them as follows.

`worker/all` only:

- `ProvideAuthCacheInvalidationWorker`
- `ProvideBatchImageCleanupService`
- `ProvideBatchImageWorkerRuntime`
- `ProvideTokenRefreshService`
- `ProvideDashboardAggregationService`
- `ProvideUsageCleanupService`
- `ProvideAccountExpiryService`
- `ProvideProxyExpiryService`
- `ProvideSubscriptionExpiryService`
- `ProvideSchedulerSnapshotService`
- `ProvideOpsMetricsCollector`
- `ProvideOpsAggregationService`
- `ProvideOpsAlertEvaluatorService`
- `ProvideOpsCleanupService`
- `ProvideIdempotencyCleanupService`
- `ProvideScheduledTestRunnerService`
- `ProvideOpsScheduledReportService`
- `ProvideBackupService`
- `ProvideUpstreamBillingProbeService`
- `ProvideOllamaCloudUsageService`
- `ProvidePaymentOrderExpiryService`
- `ProvideChannelMonitorRunner`
- `ProvideAccountMonitorRunner`
- `ProvideUserPlatformQuotaUsageFlusher`
- SettingService startup migrations and OpsService startup warm-up/refresh jobs.

`api/all` request-local:

- email queue delivery for requests accepted by the current instance;
- billing cache write workers;
- usage-record worker pool;
- content-moderation request workers;
- subscription and API-key cache invalidation subscribers;
- TimingWheel and DeferredService last-used flush;
- current-instance audit/system-log writers when their queues contain only current-instance writes.

If a listed request-local service actually consumes a shared durable queue, classify it as singleton and record the code evidence in the Task 2 report. Do not change its classification silently.

- [ ] **Step 4: Add provider-level regression tests**

Use the smallest existing fake-capable providers to prove one singleton does not call `Start()` for `api` and one request-local service still calls `Start()` for `api`. Assert observable lifecycle state or emitted work, not source text.

- [ ] **Step 5: Write failing Prompt Audit role tests**

Use fake `ConfigStore` and fake runner hooks to prove:

```go
Start(ctx, PromptStartMode{ConsumeSharedQueue: false}) // starts config; does not start runner; Enqueue works
Start(ctx, PromptStartMode{ConsumeSharedQueue: true})  // starts config and runner
Shutdown(ctx)                                         // stops exactly what was started
```

- [ ] **Step 6: Run Prompt Audit tests and verify RED**

```bash
go -C upstream/sub2api/backend test ./internal/securityaudit -run 'PromptService.*Role|PromptService.*Start' -count=1
```

Expected: compile failure because `PromptStartMode` and the new signature do not exist.

- [ ] **Step 7: Implement Prompt Audit split and main wiring**

Call:

```go
app.PromptAudit.Start(ctx, securityaudit.PromptStartMode{
    ConsumeSharedQueue: cfg.Server.ProcessRole.RunsSingletonJobs(),
})
```

The API role must retain a background context for asynchronous enqueue even though the shared runner is disabled.

- [ ] **Step 8: Regenerate Wire and run focused tests**

```bash
cd upstream/sub2api/backend
go generate ./cmd/server
go test ./internal/service ./internal/securityaudit ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 2**

```bash
git add upstream/sub2api/backend/internal/service \
  upstream/sub2api/backend/internal/securityaudit \
  upstream/sub2api/backend/cmd/server/main.go \
  upstream/sub2api/backend/cmd/server/wire_gen.go
git commit -m "feat: isolate Sub2API singleton workers"
```

---

### Task 3: Define the Permanent Blue/Green/Worker Topology

**Files:**

- Modify: `infra/compose.yaml`
- Modify: `infra/Caddyfile`
- Modify: `infra/sub2api-candidate-loader.env.example`
- Create: `tests/operations/sub2api_blue_green_topology_test.sh`

**Interfaces:**

- Compose services: `sub2api-blue`, `sub2api-green`, `sub2api-worker`.
- Release environment keys:

```dotenv
SUB2API_BLUE_IMAGE=<repository>@sha256:<64 lowercase hex>
SUB2API_GREEN_IMAGE=<repository>@sha256:<64 lowercase hex>
SUB2API_WORKER_IMAGE=<repository>@sha256:<64 lowercase hex>
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
```

- Caddy external and internal Sub2API proxies use `{$SUB2API_ACTIVE_UPSTREAM:sub2api-blue:8080}`.
- Caddy listens on internal-only Docker port `8081`; relay-ops uses `http://caddy:8081`.

- [ ] **Step 1: Write a failing rendered-topology test**

The test creates a complete temporary env, runs:

```bash
docker compose --env-file "$fixture/secret.env" --env-file "$fixture/release.env" \
  -f infra/compose.yaml config --format json
```

Parse JSON with Ruby or jq and assert:

- both slots share the same database, Redis, data bind, network/project, and request-serving environment;
- both slots have `SERVER_PROCESS_ROLE=api` and distinct image variables;
- worker has `SERVER_PROCESS_ROLE=worker`, no public Caddy dependency, and the same shared data inputs;
- relay-ops points to `http://caddy:8081`;
- Caddy has no dependency on relay-ops or either API slot becoming healthy;
- Caddy environment contains exactly one allowlisted active upstream.

Also run Caddy validation twice in the project Caddy image, once per allowed upstream, and assert `sub2api:8080` no longer appears in the adapted proxy routes.

- [ ] **Step 2: Run the topology test and verify RED**

```bash
bash tests/operations/sub2api_blue_green_topology_test.sh
```

Expected: FAIL because the three-service topology and variable upstream do not exist.

- [ ] **Step 3: Implement Compose anchors and Caddy routing**

Create a shared Sub2API environment anchor, but keep each service image and role explicit. Add Caddy `:8081` with only the active Sub2API reverse proxy. Replace every public Sub2API proxy target with the active-upstream placeholder. Remove the relay-ops/Caddy dependency cycle.

- [ ] **Step 4: Run topology and existing Compose/Caddy tests**

```bash
bash tests/operations/sub2api_blue_green_topology_test.sh
bash upstream/sub2api/deploy/test-caddyfile-cache.sh
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```bash
git add infra/compose.yaml infra/Caddyfile infra/sub2api-candidate-loader.env.example \
  tests/operations/sub2api_blue_green_topology_test.sh
git commit -m "feat: define Sub2API blue green topology"
```

---

### Task 4: Implement the Fail-Closed Host Blue-Green Executor

**Files:**

- Create: `ops/deploy-sub2api-blue-green-host.sh`
- Create: `tests/operations/deploy_sub2api_blue_green_host_test.sh`

**Interfaces:**

Command:

```bash
bash ops/deploy-sub2api-blue-green-host.sh \
  --mode rehearsal|production \
  --image <repository>@sha256:<64 lowercase hex> \
  --source-commit <40 lowercase hex> \
  --source-tree <40 lowercase hex> \
  --tested-tree <40 lowercase hex> \
  --migrations-hash <64 lowercase hex>
```

Required production environment:

```dotenv
DEPLOY_ROOT=/absolute/path
BASE_COMPOSE=/absolute/path/compose.yaml
SECRET_ENV=/absolute/path/secret.env
RELEASE_ENV=/absolute/path/release.env
RELEASE_STATE=/absolute/path/blue-green-state.json
RELEASE_RECORD_ROOT=/absolute/path/release-records
BASE_URL=https://example.invalid
ADMIN_API_KEY_FILE=/absolute/path/admin-key
GATEWAY_API_KEY_FILE=/absolute/path/gateway-key
```

Machine-readable gate output:

```json
{
  "schema_version": 1,
  "downtime_required": true,
  "reason_code": "migration_set_changed",
  "reason": "candidate migration set differs from the active release",
  "estimated_unavailable_seconds": 300,
  "rollback": ["keep current active slot", "do not start candidate", "prepare an authorized maintenance release"]
}
```

- [ ] **Step 1: Build the fake command harness and write RED tests for validation**

Fake `docker`, `curl`, `jq`, `flock`/lock directory behavior, and time. Cover malformed digest, label mismatch, source/tested tree mismatch, non-Linux production, non-default Docker context, symlink paths, duplicate/invalid state keys, stale partial record, and concurrent release lock.

- [ ] **Step 2: Write RED tests for downtime gates**

Cover migration hash change, legacy topology bootstrap, insufficient disk/memory/DB connection headroom, invalid active slot/upstream pair, shared container identity mismatch, and candidate role not equal to `api`. Assert no `docker compose up`, Caddy reload, or release-env write occurs before the JSON gate is printed.

- [ ] **Step 3: Write RED tests for the successful command order**

Assert the observable sequence:

```text
inspect immutable image labels
capture postgres/redis/caddy identities
pull inactive-slot image
up --no-deps inactive-slot
Docker-network candidate health/version/auth/gateway checks
caddy validate with candidate upstream
caddy reload with candidate upstream
public acceptance
atomic state/release-env write
up --no-deps --force-recreate sub2api-worker
worker health/log acceptance
final identity check
atomic success record
```

The test rejects `compose down`, volume deletion, database restore, PostgreSQL/Redis/Caddy recreation, or stopping the previous API slot.

- [ ] **Step 4: Write RED rollback and interruption-recovery tests**

Cover candidate failure before cutover, Caddy validate failure, reload failure, public acceptance failure, worker update failure, and a process restart with a partial record. Public or worker failure after cutover must reload the previous upstream and restore the prior worker digest before final failure is recorded.

- [ ] **Step 5: Run the host executor tests and verify RED**

```bash
bash tests/operations/deploy_sub2api_blue_green_host_test.sh
```

Expected: FAIL because the host executor does not exist.

- [ ] **Step 6: Implement minimal executor behavior**

Use an atomic `0600` JSON state file and per-attempt `.partial` record. Validate the Caddy upstream through an exact shell `case` allowlist. Pass the selected upstream only to `caddy validate`/`caddy reload` process environments, then atomically persist the same value for future container restarts.

- [ ] **Step 7: Run host executor and legacy operation tests**

```bash
bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash tests/operations/deploy_sub2api_release_test.sh
bash tests/operations/update_sub2api_host_test.sh
```

Expected: PASS; legacy scripts remain behaviorally unchanged.

- [ ] **Step 8: Commit Task 4**

```bash
git add ops/deploy-sub2api-blue-green-host.sh \
  tests/operations/deploy_sub2api_blue_green_host_test.sh
git commit -m "feat: add fail closed blue green host deployer"
```

---

### Task 5: Add Tested-Tree Evidence and the 30-Minute Release Controller

**Files:**

- Create: `ops/write-sub2api-test-evidence.sh`
- Create: `ops/release-sub2api-blue-green.sh`
- Create: `tests/operations/release_sub2api_blue_green_test.sh`
- Modify: `.github/workflows/sub2api-release-preparation.yml`

**Interfaces:**

Test evidence schema:

```json
{
  "schema_version": 1,
  "source_commit": "<40 lowercase hex>",
  "tested_tree": "<40 lowercase hex>",
  "migrations_hash": "<64 lowercase hex>",
  "created_at": "<RFC3339 UTC>",
  "commands": ["<exact completed command>"],
  "result": "passed"
}
```

Release command:

```bash
bash ops/release-sub2api-blue-green.sh \
  --mode rehearsal|production \
  --evidence /absolute/path/test-evidence.json
```

Image labels:

```text
com.xingqiao.sub2api.qualified=true
com.xingqiao.sub2api.source.commit=<40 hex>
com.xingqiao.sub2api.source.tree=<40 hex>
com.xingqiao.sub2api.tested.tree=<40 hex>
com.xingqiao.sub2api.migrations.sha256=<64 hex>
```

- [ ] **Step 1: Write RED evidence validation tests**

Cover dirty tree, tested-tree mismatch, commit mismatch, migration hash mismatch, failed result, missing command evidence, permissive/symlink evidence file, malformed JSON, and unknown keys. Prove validation happens before `docker buildx` or any SSH/host call.

- [ ] **Step 2: Write RED build/publish/timeout tests**

Fake Docker Buildx, registry/publish transport, SSH candidate loader, and monotonic time. Assert Linux AMD64, exact labels, immutable digest resolution, per-stage timeout, and total 1800-second budget. The controller must propagate host `downtime_required=true` without retrying or mutating production.

- [ ] **Step 3: Run release controller tests and verify RED**

```bash
bash tests/operations/release_sub2api_blue_green_test.sh
```

Expected: FAIL because the evidence writer and release controller do not exist.

- [ ] **Step 4: Implement evidence writer and controller**

The evidence writer accepts repeated `--command` arguments only after those commands have completed successfully in the calling verification wrapper. The controller computes `git rev-parse HEAD^{tree}` and the migration-set hash independently, validates the evidence, builds once, publishes once, resolves the digest, and invokes the host executor once.

- [ ] **Step 5: Extend qualified-image workflow labels**

Compute source tree and migrations hash after qualification and add the five required labels. Keep current upstream version/commit labels. Do not trigger production deployment from the workflow.

- [ ] **Step 6: Run release and workflow tests**

```bash
bash tests/operations/release_sub2api_blue_green_test.sh
ruby -Itest tests/operations/sub2api_release_workflow_test.rb
ruby -Itest tests/operations/publish_sub2api_candidate_test.rb
git diff --check
```

Expected: PASS.

- [ ] **Step 7: Commit Task 5**

```bash
git add ops/write-sub2api-test-evidence.sh ops/release-sub2api-blue-green.sh \
  tests/operations/release_sub2api_blue_green_test.sh \
  .github/workflows/sub2api-release-preparation.yml
git commit -m "feat: add tested tree blue green release controller"
```

---

### Task 6: Rehearse Both Slots and Publish the Operator Handoff

**Files:**

- Modify: `infra/compose.sub2api-rehearsal.yaml`
- Modify: `tests/operations/sub2api_blue_green_topology_test.sh`
- Modify: `tests/operations/deploy_sub2api_blue_green_host_test.sh`
- Create: `docs/runbooks/sub2api-blue-green-production-deployment.md`
- Create: `docs/superpowers/reports/2026-07-31-command-driven-blue-green-local-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**

- Rehearsal project name: `sub2api-blue-green-rehearsal`.
- Rehearsal runs isolated PostgreSQL and Redis storage; it never mounts production paths.
- The runbook production command remains inert documentation until the user later issues the deployment instruction.

- [ ] **Step 1: Write the failing two-slot rehearsal assertions**

Extend the operation tests to run or fake a complete blue-to-green and green-to-blue cycle. Assert:

- candidate failure leaves the active slot serving;
- Caddy reload failure leaves the active upstream unchanged;
- public acceptance failure cuts back to the previous slot;
- PostgreSQL, Redis, and Caddy container IDs are unchanged;
- only one worker role is running;
- the previous API slot remains running after success;
- fixture elapsed time remains below 1800 seconds.

- [ ] **Step 2: Run rehearsal tests and verify RED**

```bash
bash tests/operations/sub2api_blue_green_topology_test.sh
bash tests/operations/deploy_sub2api_blue_green_host_test.sh
```

Expected: FAIL because the rehearsal topology still has one application slot.

- [ ] **Step 3: Implement the isolated rehearsal topology**

Mirror the permanent blue/green/worker roles while preserving isolated storage and localhost-only ports. Include a deterministic test hook that causes the candidate health or public acceptance stage to fail without changing production code.

- [ ] **Step 4: Write the runbook and verification report**

The runbook must contain:

- exact future `部署生产` execution prerequisites and command;
- the difference between ordinary production authorization and `允许停机部署`;
- the first-topology-bootstrap gate;
- successful stage timeline and 1800-second hard stop;
- all `downtime_required` reason codes;
- cutback and interrupted-release recovery steps;
- proof that shared services are never rebuilt;
- state/record file ownership and retention.

The verification report records every command, exit status, observed duration, unverified production assumptions, and the exact reason the project remains `待生产验收`.

- [ ] **Step 5: Run the full local verification matrix**

```bash
go -C upstream/sub2api/backend test ./... -count=1
go -C upstream/sub2api/backend vet ./...
go -C sub2api-updater test ./... -count=1
go -C sub2api-updater vet ./...
go -C relay-ops-service test ./... -count=1
go -C relay-ops-service vet ./...
bash tests/operations/sub2api_blue_green_topology_test.sh
bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash tests/operations/release_sub2api_blue_green_test.sh
bash tests/operations/deploy_sub2api_release_test.sh
bash tests/operations/update_sub2api_host_test.sh
ruby -Itest tests/operations/sub2api_release_workflow_test.rb
ruby -Itest tests/operations/publish_sub2api_candidate_test.rb
bash upstream/sub2api/deploy/test-caddyfile-cache.sh
git diff --check
```

Expected: every command exits 0.

- [ ] **Step 6: Update durable project state**

Set the initiative to `准备完成（待生产部署与验收）`, link the design, plan, runbook, and verification report, and state explicitly that no production connection, push, deployment, or live validation occurred in this implementation loop.

- [ ] **Step 7: Commit Task 6**

```bash
git add infra/compose.sub2api-rehearsal.yaml \
  tests/operations/sub2api_blue_green_topology_test.sh \
  tests/operations/deploy_sub2api_blue_green_host_test.sh \
  docs/runbooks/sub2api-blue-green-production-deployment.md \
  docs/superpowers/reports/2026-07-31-command-driven-blue-green-local-verification.md \
  docs/project/current-state.md docs/project/project-progress.md
git commit -m "docs: hand off Sub2API blue green deployment"
```

---

## Final Review and Completion Gate

- [ ] Generate a full branch review package from the worktree base to `HEAD`.
- [ ] Dispatch an independent whole-branch reviewer focused on process-role isolation, zero-write API startup, rollback ordering, downtime gates, command injection, secret handling, state atomicity, and preservation of shared container identity.
- [ ] Resolve every Critical or Important finding and re-run the affected tests.
- [ ] Re-run the complete Task 6 verification matrix after the final fix.
- [ ] Confirm `git status --short` contains only intentional task files.
- [ ] Confirm no production host was contacted and no branch was pushed unless the user separately authorized it.
- [ ] Leave the progress ledger at `准备完成（待生产部署与验收）`, not `已完成`.
