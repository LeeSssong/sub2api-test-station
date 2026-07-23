# Feishu Shared Aliu Backup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Safely configure Aliu account `2` as the shared dry-run backup for both production public groups.

**Architecture:** Relax only the backup-uniqueness rule while preserving unique primaries and disjoint primary/backup roles. Replace the group-only PostgreSQL advisory lock with deterministic namespaced route-resource locks covering the group and both accounts.

**Tech Stack:** Go 1.24.13, PostgreSQL advisory transaction locks, Sub2API v0.1.161 native Admin API, Docker Compose.

## Global Constraints

- Use only Sub2API `v0.1.161` native Admin API methods; never write Sub2API PostgreSQL directly.
- Never send `confirm_mixed_channel_risk`.
- Keep account `9` present, unbound and unschedulable.
- Keep `RELAY_OPS_MODE=read_only` and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`.
- Recreate only `relay-ops`; do not recreate Sub2API, PostgreSQL, Redis or Caddy.
- Do not perform a real route switch or send invitations.

---

### Task 1: Accept Shared Backup Configuration

**Files:**
- Modify: `relay-ops-service/internal/routingcontrol/controller.go`
- Test: `relay-ops-service/internal/routingcontrol/controller_test.go`

**Interfaces:**
- Consumes: `LoadConfig(path string) (Config, error)`
- Produces: validated `Config` allowing the same positive account ID in backup-only roles

- [ ] **Step 1: Write failing configuration tests**

Add a valid route with backup `12` reused by both groups. Add invalid cases for duplicate primary IDs and for a primary ID reused as any backup.

- [ ] **Step 2: Verify RED**

Run `go test ./internal/routingcontrol -run 'LoadConfig' -count=1` from `relay-ops-service`. Expected: the shared-backup acceptance test fails with `routing account IDs must not be reused`.

- [ ] **Step 3: Implement role-aware validation**

Collect primary and backup ID sets. Reject duplicate primary IDs and the intersection of primary and backup sets. Do not reject duplicates inside the backup set.

- [ ] **Step 4: Verify GREEN**

Run `go test ./internal/routingcontrol -run 'LoadConfig' -count=1`. Expected: PASS.

### Task 2: Lock Shared Route Resources

**Files:**
- Modify: `relay-ops-service/internal/commands/worker.go`
- Modify: `relay-ops-service/internal/app/feishu_commands.go`
- Modify: `relay-ops-service/internal/app/feishu_commands_test.go`
- Modify: `relay-ops-service/internal/store/feishu_commands.go`
- Test: `relay-ops-service/internal/commands/worker_test.go`
- Test: `relay-ops-service/internal/store/feishu_commands_test.go`

**Interfaces:**
- Produces: `commands.RouteLockIDs{GroupID, PrimaryAccountID, BackupAccountID int64}`
- Produces: `WithFeishuRouteLock(context.Context, commands.RouteLockIDs, func(context.Context) commands.Completion) (commands.Completion, error)`

- [ ] **Step 1: Write failing worker and store tests**

Worker tests must assert all three IDs reach the repository. Store tests must start two locks with different group/primary IDs but the same backup ID and prove the second callback blocks. A third route with no shared IDs must enter concurrently.

- [ ] **Step 2: Verify RED**

Run `go test ./internal/commands ./internal/store ./internal/app -run 'RouteLock|SharedAccount' -count=1`. Expected: compile failure because the new lock interface does not exist.

- [ ] **Step 3: Implement deterministic resource locks**

Validate all IDs are positive. Build `relay_ops_feishu_group:<id>` and `relay_ops_feishu_account:<id>` keys, sort and deduplicate them, then acquire `pg_advisory_xact_lock(hashtextextended($1, 0))` for each key inside one transaction before invoking the callback.

- [ ] **Step 4: Wire route IDs from configuration**

Build the worker's route-lock map from each `GroupRoute`. Do not expose dynamic lock IDs from the incoming command.

- [ ] **Step 5: Verify GREEN**

Run `go test ./internal/commands ./internal/store ./internal/app -count=1`. Expected: PASS.

### Task 3: Preserve Shared Aliu Bindings

**Files:**
- Test: `relay-ops-service/internal/routingcontrol/controller_test.go`
- Verify: `relay-ops-service/internal/routingcontrol/controller.go`

**Interfaces:**
- Consumes: `addGroup([]int64, int64) []int64` and `removeGroup([]int64, int64) []int64`

- [ ] **Step 1: Add shared-account switching regression test**

Use one backup account for both routes. Switch one group to backup and restore it while the other group remains bound; assert the other group ID remains in the backup account after both operations.

- [ ] **Step 2: Verify behavior**

Run `go test ./internal/routingcontrol -run 'SharedBackup' -count=1`. Expected: PASS with the existing additive/removal helpers; if it fails, make only the minimal helper correction and rerun.

### Task 4: Full Local Verification

**Files:**
- Verify: `relay-ops-service/**`
- Verify: `tests/relay_ops/validate_relay_ops_contract.sh`

- [ ] **Step 1: Format**

Run `gofmt -w` on the modified Go files.

- [ ] **Step 2: Run race tests in Go 1.24.13**

Run the pinned Go container with `go test ./... -race -count=1`. Expected: all packages pass.

- [ ] **Step 3: Run vet**

Run the pinned Go container with `go vet ./...`. Expected: exit `0`.

- [ ] **Step 4: Run deployment contract**

Run `bash tests/relay_ops/validate_relay_ops_contract.sh`. Expected: `relay-ops contract: ok`.

### Task 5: Production Dry-Run Deployment

**Files:**
- Modify on server: `/opt/sub2api/secrets/feishu-routing.json`
- Rebuild on server: `/opt/sub2api/production/relay-ops-service`
- Create: `docs/superpowers/reports/2026-07-20-feishu-shared-aliu-dry-run-verification.md`

- [ ] **Step 1: Freeze pre-change evidence**

Record modes, container IDs, relay-ops health, route/account redacted state and normalized hashes. Confirm account `2` native test is successful and account `9` is unbound/unschedulable.

- [ ] **Step 2: Deploy source and routing config**

Install only the modified relay-ops sources and a `0600/0640` routing file mapping Pro `7 -> 2` and Plus `8 -> 2`. Do not change command mode.

- [ ] **Step 3: Build and recreate only relay-ops**

Run Compose build for `relay-ops`, validate Compose, and run `docker compose up -d --no-deps --force-recreate relay-ops`. Confirm all other container IDs are unchanged.

- [ ] **Step 4: Verify dry-run state**

Confirm `read_only/dry_run`, health endpoints, both routes currently `primary`, account `9` remains unbound/unschedulable, and route snapshot hashes are unchanged. Exercise only dry-run/status behavior; do not enter `enabled`.

- [ ] **Step 5: Record evidence and handoff**

Document image/container IDs, tests, modes, routing mapping, zero-write hashes, remaining invitation/configuration adjustments and the separate approval required for any real switch.

