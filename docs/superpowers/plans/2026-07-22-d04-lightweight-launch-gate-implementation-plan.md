# D04 Lightweight Launch Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the historical provider-specific D04 opening decision with a provider-neutral v2 evaluator and a lightweight, verified server-local backup of authoritative account data.

**Architecture:** Keep v1 files immutable as historical evidence. Add a strict, secret-free Ruby v2 policy evaluator; add a SQLite online-backup subcommand to the existing D04 binary; orchestrate PostgreSQL and D04 snapshots with one portable Bash command that atomically publishes only verified sets. Migrate launch documentation and the D04 cost-policy identifier to generic `active_upstream` terminology, then perform read-only production acceptance.

**Tech Stack:** Ruby 3 / Minitest / YAML / JSON, Go 1.24 / `database/sql` / `modernc.org/sqlite`, Bash 3.2+, Docker Compose, PostgreSQL 18 `pg_dump -Fc`, SHA-256.

## Global Constraints

- Policy ID is exactly `D04-LIGHTWEIGHT-LAUNCH-v2`; v1 policy, evaluator, tests, and reports remain historical and unchanged.
- V2 policy fields, snapshot fields, blocking reasons, required actions, alerts, cost-policy IDs, and evaluator branches must not contain a concrete provider name.
- Active-upstream minimum balance starts at USD 10 and remains policy-configurable.
- Quality uses a 15-minute natural-production-traffic window, evidence no older than 20 minutes, at least 20 samples, success rate at least 95%, error rate at most 5%, TTFT P95 at most 5000 ms, and total-latency P95 at most 45000 ms.
- D04 launch invariants remain 15 users, USD 20 daily login credit, USD 100 total cost-risk budget, and 1000 bps.
- Account backup is server-local, no older than 24 hours, SHA-256 verified, and includes the complete Sub2API PostgreSQL database plus a consistent D04 SQLite snapshot.
- Keep the newest three verified local backup sets; retention count is not an evaluator gate.
- Do not add off-site backup, encryption, seven-day retention, balance-runway days, spend-rate, or recurring restore-drill requirements.
- Evaluator is offline and report-only: `real_action_executed=false`, `external_system_contacted=false`.
- Production remains D04 `read_only/registration=false` and relay-ops `read_only/dry_run` until a v2 `go`; no route, multiplier, price, balance, Key, candidate, probe, account binding, model request, or synthetic Feishu event change is allowed.

---

### Task 1: Provider-neutral v2 readiness evaluator

**Files:**
- Create: `config/operations/D04-lightweight-launch-readiness-v2.yaml`
- Create: `config/operations/d04-lightweight-launch-snapshot.example.yaml`
- Create: `ops/evaluate-d04-lightweight-launch-readiness.rb`
- Create: `tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb`

**Interfaces:**
- Consumes: two secret-free YAML documents and optional `D04_LAUNCH_NOW` ISO-8601 timestamp.
- Produces: `D04LightweightLaunchReadiness::Evaluator#evaluate(snapshot) -> Hash` and CLI JSON with `decision`, `blocking_reasons`, `required_actions`, `policy`, `derived`, `real_action_executed`, and `external_system_contacted`.

- [x] **Step 1: Write the provider-neutral policy and snapshot fixtures**

Create the exact schema from the approved design. The snapshot must use only these top-level keys:

```ruby
%w[
  schema_version snapshot_id status captured_at approvals modes services
  d04 active_upstream account_backup operations
]
```

Use `launch_approved` as the only approval and `quality_source: natural_production_traffic` as the only accepted quality source.

- [x] **Step 2: Write failing validator and evaluator tests**

The healthy fixture must produce `go` and use generic sections:

```ruby
def test_complete_fresh_snapshot_is_go_and_executes_nothing
  result = evaluate
  assert_equal "go", result.fetch("decision")
  assert_empty result.fetch("blocking_reasons")
  assert_equal false, result.fetch("real_action_executed")
  assert_equal false, result.fetch("external_system_contacted")
end
```

Add focused tests for every v2 reason code, strict unknown-key rejection, future timestamps, forbidden credential keys/values, D04 configuration mismatch, user-limit overflow, natural-source enforcement, insufficient-sample percentile suppression, backup freshness/hash/scope, CLI output, and provider-neutral source scanning.

- [x] **Step 3: Run the new tests and verify RED**

Run:

```bash
ruby tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
```

Expected: failure because `ops/evaluate-d04-lightweight-launch-readiness.rb` and the v2 module do not exist.

- [x] **Step 4: Implement strict validators and the report-only evaluator**

Implement a separate module:

```ruby
module D04LightweightLaunchReadiness
  class ValidationError < StandardError; end
  class PolicyValidator < ValidatorBase; end
  class SnapshotValidator < ValidatorBase; end
  class Evaluator
    def initialize(policy, now: Time.now); end
    def evaluate(snapshot); end
  end
end
```

Use exact allowed-key sets at every mapping level. Syntax-invalid evidence raises `ValidationError`; valid stale or insufficient evidence returns `no_go`. Reject timestamps later than evaluation time instead of clamping their age to zero. Evaluate percentile thresholds only when `sample_count >= samples_min`.

Use provider-neutral mappings such as:

```ruby
ACTIONS = {
  "launch_not_approved" => "record_launch_approval",
  "upstream_balance_unknown" => "refresh_upstream_financial_evidence",
  "upstream_balance_below_minimum" => "replenish_active_upstream_balance",
  "upstream_quality_metrics_stale" => "refresh_upstream_quality_metrics",
  "account_backup_stale" => "create_verified_local_account_backup",
  "rollback_unverified" => "verify_registration_rollback"
}.freeze
```

- [x] **Step 5: Run v2 and historical v1 tests**

```bash
ruby tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
ruby tests/operations/evaluate_d04_launch_readiness_test.rb
```

Expected: both suites pass; v1 historical behavior remains unchanged.

- [x] **Step 6: Commit the evaluator slice**

```bash
git add config/operations/D04-lightweight-launch-readiness-v2.yaml \
  config/operations/d04-lightweight-launch-snapshot.example.yaml \
  ops/evaluate-d04-lightweight-launch-readiness.rb \
  tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
git commit -m "feat: add lightweight D04 launch evaluator"
```

### Task 2: Consistent D04 SQLite backup command

**Files:**
- Modify: `internal-test-service/internal/store/sqlite.go`
- Modify: `internal-test-service/internal/store/sqlite_test.go`
- Modify: `internal-test-service/cmd/internal-test-service/main.go`
- Create: `internal-test-service/cmd/internal-test-service/main_test.go`

**Interfaces:**
- Produces: `store.BackupSQLite(ctx context.Context, sourcePath, destinationPath string) error`.
- Produces: `runBackupCommand(args []string, getenv func(string) string, stderr io.Writer) (handled bool, exitCode int)` for a testable pre-config CLI branch.
- Produces CLI: `internal-test-service backup-sqlite SOURCE DESTINATION`; it exits nonzero without starting the HTTP service when backup fails.

- [x] **Step 1: Write a failing WAL-consistency test**

Create a source database in WAL mode, insert a committed row without manually checkpointing the WAL, call `BackupSQLite`, then open the destination and require `PRAGMA integrity_check = ok` and the inserted row to exist:

```go
func TestBackupSQLiteIncludesCommittedWALState(t *testing.T) {
    err := BackupSQLite(context.Background(), sourcePath, destinationPath)
    requireNoError(t, err)
    assertIntegrityAndExpectedRow(t, destinationPath)
}
```

Also require failure when destination already exists so a backup cannot overwrite a verified artifact.

- [x] **Step 2: Run focused Go tests and verify RED**

```bash
go test ./internal/store ./cmd/internal-test-service -count=1
```

Expected: compile failure because `BackupSQLite` and the backup CLI branch do not exist. Use the repository's fixed Go 1.24 container if local Go is unavailable.

- [x] **Step 3: Implement SQLite online backup**

Open the source through the existing SQLite driver with a read-only connection and busy timeout, reject an existing destination, and execute SQLite's consistent backup operation:

```go
func BackupSQLite(ctx context.Context, sourcePath, destinationPath string) error {
    if !filepath.IsAbs(sourcePath) || !filepath.IsAbs(destinationPath) {
        return errors.New("backup paths must be absolute")
    }
    if filepath.Clean(sourcePath) == filepath.Clean(destinationPath) {
        return errors.New("backup destination must differ from source")
    }
    if _, err := os.Stat(destinationPath); err == nil {
        return errors.New("backup destination already exists")
    } else if !errors.Is(err, os.ErrNotExist) {
        return fmt.Errorf("inspect backup destination: %w", err)
    }

    sourceURL := &url.URL{Scheme: "file", Path: sourcePath}
    query := sourceURL.Query()
    query.Set("mode", "ro")
    query.Set("_pragma", "busy_timeout(5000)")
    sourceURL.RawQuery = query.Encode()
    db, err := sql.Open("sqlite", sourceURL.String())
    if err != nil {
        return fmt.Errorf("open backup source: %w", err)
    }
    defer db.Close()
    db.SetMaxOpenConns(1)
    if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, destinationPath); err != nil {
        return fmt.Errorf("create sqlite backup: %w", err)
    }

    copyDB, err := sql.Open("sqlite", "file:"+destinationPath+"?mode=ro")
    if err != nil {
        return fmt.Errorf("open sqlite backup: %w", err)
    }
    defer copyDB.Close()
    var integrity string
    if err := copyDB.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
        return fmt.Errorf("verify sqlite backup: %w", err)
    }
    if integrity != "ok" {
        return fmt.Errorf("verify sqlite backup: %s", integrity)
    }
    return nil
}
```

The command path must run before `config.Load`, accept exactly two path arguments, and print only a generic error unless `D04_DEBUG=1`. `main` calls `runBackupCommand(os.Args[1:], os.Getenv, os.Stderr)` first; when `handled` is true it exits with the returned code and never constructs the HTTP application.

- [x] **Step 4: Run focused and full D04 tests**

```bash
go test ./internal/store ./cmd/internal-test-service -count=1
go test ./... -count=1
go test ./... -race -count=1
go vet ./...
```

Expected: all pass.

- [x] **Step 5: Commit the SQLite backup slice**

```bash
git add internal-test-service/internal/store/sqlite.go \
  internal-test-service/internal/store/sqlite_test.go \
  internal-test-service/cmd/internal-test-service/main.go \
  internal-test-service/cmd/internal-test-service/main_test.go
git commit -m "feat: add consistent D04 SQLite backup"
```

### Task 3: Atomic server-local account backup set

**Files:**
- Create: `ops/backup-d04-account-data.sh`
- Create: `tests/operations/backup_d04_account_data_test.sh`
- Modify: `.gitignore`

**Interfaces:**
- Consumes environment: `D04_BACKUP_ROOT`, `SUB2API_COMPOSE_FILE`, `D04_IMAGE`, and `D04_VOLUME`; defaults target `/opt/sub2api/production` resources.
- Produces one promoted directory named by UTC timestamp containing `sub2api.dump`, `d04.sqlite`, `SHA256SUMS`, and `metadata.json`.

- [x] **Step 1: Write a fake-Docker shell test**

Put a fake `docker` executable first in `PATH`. It must emit deterministic PostgreSQL dump bytes for `docker compose ... exec -T postgres` and deterministic SQLite bytes for `docker run ... backup-sqlite`. Assert:

```bash
[[ -f "$set/sub2api.dump" ]]
[[ -f "$set/d04.sqlite" ]]
[[ -f "$set/SHA256SUMS" ]]
[[ -f "$set/metadata.json" ]]
(cd "$set" && sha256sum -c SHA256SUMS)
[[ "$(stat_mode "$set")" == 700 ]]
[[ "$(stat_mode "$set/sub2api.dump")" == 600 ]]
```

Add cases for Docker failure leaving no promoted directory, existing lock refusal, and four verified fixtures being pruned to the newest three without touching unrelated files.

- [x] **Step 2: Run the shell test and verify RED**

```bash
bash tests/operations/backup_d04_account_data_test.sh
```

Expected: failure because `ops/backup-d04-account-data.sh` does not exist.

- [x] **Step 3: Implement the portable backup command**

Use `set -euo pipefail`, `umask 077`, and a `mkdir` lock directory instead of platform-specific `flock`. Create the set under a validated temporary child of the configured backup root. Run:

```bash
docker compose -f "$SUB2API_COMPOSE_FILE" exec -T postgres \
  sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc'

docker run --rm --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true --user 0:0 \
  --mount "type=volume,src=$D04_VOLUME,dst=/var/lib/internal-test,readonly" \
  --mount "type=bind,src=$temporary_set,dst=/backup" \
  "$D04_IMAGE" backup-sqlite \
  /var/lib/internal-test/internal-test.db /backup/d04.sqlite
```

Generate and verify `SHA256SUMS`, write secret-free metadata, atomically rename the temporary set, and only then remove validated timestamp directories beyond the newest three. Never use the backup root, a volume path, or an unresolved variable as a deletion target.

- [x] **Step 4: Add local snapshot ignore rule and rerun tests**

Add:

```gitignore
config/operations/d04-lightweight-launch-snapshot.local.yaml
```

Run:

```bash
bash tests/operations/backup_d04_account_data_test.sh
bash -n ops/backup-d04-account-data.sh
```

Expected: pass.

- [x] **Step 5: Commit the backup orchestration slice**

```bash
git add .gitignore ops/backup-d04-account-data.sh \
  tests/operations/backup_d04_account_data_test.sh
git commit -m "feat: add lightweight local account backup"
```

### Task 4: Provider-neutral launch runtime and operator documentation

**Files:**
- Modify: `infra/compose.d04-launch.yaml`
- Modify: `tests/internal_test/validate_internal_test_contract.sh`
- Modify: `docs/runbooks/operations-and-incident-response.md`
- Modify: `docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md`

**Interfaces:**
- Consumes: v2 policy/evaluator and local backup command from Tasks 1-3.
- Produces: a launch overlay with generic `D04_COST_POLICY_ID` and an operator flow that invokes only v2.

- [x] **Step 1: Write failing contract assertions**

Require:

```bash
require 'D04_COST_POLICY_ID: d04-active-upstream-conservative-1000bps-v2' infra/compose.d04-launch.yaml
require 'D04-LIGHTWEIGHT-LAUNCH-v2' docs/runbooks/operations-and-incident-response.md
require 'backup-d04-account-data.sh' docs/runbooks/operations-and-incident-response.md
```

Reject concrete provider names in `infra/compose.d04-launch.yaml`, the v2 policy/snapshot/evaluator, and the active D04 launch sections of the runbook/checklist. Do not scan historical v1 evidence.

- [x] **Step 2: Run the contract and verify RED**

```bash
bash tests/internal_test/validate_internal_test_contract.sh
```

Expected: failure on the historical provider-specific cost-policy ID and v1 runbook command.

- [x] **Step 3: Migrate the launch overlay and runbook/checklist**

Change the cost-policy ID to:

```yaml
D04_COST_POLICY_ID: d04-active-upstream-conservative-1000bps-v2
```

Replace v1 opening instructions with the v2 evaluator, one `launch_approved` value, a fresh verified local account backup, generic active-upstream balance/quality, and no spend-runway/off-site/restore-drill gates. Keep rollback commands and all production safety boundaries.

- [x] **Step 4: Run deployment and documentation contracts**

```bash
bash tests/internal_test/validate_internal_test_contract.sh
docker compose -f infra/compose.d04-read-only.yaml \
  -f infra/compose.d04-launch.yaml config --quiet
rg -n 'D04_COST_POLICY_ID:.*(wawazz|neko|xm|aliu)' infra/compose.d04-launch.yaml
```

Expected: contracts pass; the final search has no output.

- [x] **Step 5: Commit runtime and operator migration**

```bash
git add infra/compose.d04-launch.yaml \
  tests/internal_test/validate_internal_test_contract.sh \
  docs/runbooks/operations-and-incident-response.md \
  docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md
git commit -m "docs: migrate D04 opening to lightweight gate"
```

### Task 5: Full verification and read-only production acceptance

**Files:**
- Create: `docs/superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`

**Interfaces:**
- Consumes: verified v2 files, D04 image containing `backup-sqlite`, current production health, and naturally collected upstream evidence.
- Produces: a secret-free v2 live snapshot, evaluator output, production backup evidence, and authoritative project status.

- [x] **Step 1: Run all local gates**

```bash
ruby tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
ruby tests/operations/evaluate_d04_launch_readiness_test.rb
bash tests/operations/backup_d04_account_data_test.sh
bash tests/internal_test/validate_internal_test_contract.sh
docker compose -f infra/compose.d04-read-only.yaml \
  -f infra/compose.d04-launch.yaml config --quiet
cd internal-test-service && go test ./... -race -count=1 && go vet ./...
git diff --check
```

Expected: all pass.

- [x] **Step 2: Build the D04 image and verify the backup subcommand locally**

Build the pinned AMD64 production image on the production host, record its image ID, and invoke `backup-sqlite` against a disposable fixture volume. Do not recreate production yet. Require a readable destination with SQLite `integrity_check=ok`.

- [x] **Step 3: Deploy only the D04 read-only image when required**

Update only the independent D04 Compose image tag, recreate only `internal-test-service`, and keep:

```text
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
relay-ops mode=read_only
Feishu command mode=dry_run
```

Verify D04 healthy/restart `0`, same-origin registration `403 D04_REGISTRATION_CLOSED`, one existing user, one existing grant, zero usage, and unchanged route/configuration hashes. If the one-shot backup can use the new image without recreating D04, leave the running D04 image unchanged and record that narrower path instead.

- [x] **Step 4: Create one fresh production account backup set**

Install the backup command under the restricted production directory and execute it once. Record only timestamp, sizes, SHA-256 verification status, permissions, scope flags, and count of retained sets. Do not print archives, rows, credentials, or secret file contents.

- [x] **Step 5: Generate and evaluate the v2 live snapshot**

Populate `config/operations/d04-lightweight-launch-snapshot.local.yaml` from current production health and natural traffic only. Keep `launch_approved: false` unless the user's approval explicitly covers actual opening, then run:

```bash
ruby ops/evaluate-d04-lightweight-launch-readiness.rb evaluate \
  config/operations/D04-lightweight-launch-readiness-v2.yaml \
  config/operations/d04-lightweight-launch-snapshot.local.yaml
```

Record the actual `go` or `no_go` result without lowering thresholds or manufacturing samples. Regardless of the result, keep registration closed during this task.

- [x] **Step 6: Update authority documents and verification report**

Document:

```text
v1 = historical evidence only
v2 = active provider-neutral launch gate
local backup = current lightweight account-data protection
off-site/restore-drill/retention-days = accepted non-goals, not blockers
next action = resolve only actual v2 blockers, then controlled D04 opening
```

Include production modes, health, zero route/configuration writes, backup verification, evaluator result, and exact remaining blockers. Do not replace concrete historical evidence in dated reports.

- [x] **Step 7: Final verification and commit**

Rerun the full commands from Step 1 plus:

```bash
rg -n 'wawazz|neko|xm|aliu' \
  config/operations/D04-lightweight-launch-readiness-v2.yaml \
  config/operations/d04-lightweight-launch-snapshot.example.yaml \
  ops/evaluate-d04-lightweight-launch-readiness.rb \
  infra/compose.d04-launch.yaml
git diff --check
```

Expected: no provider-name output from active v2/runtime files; all tests pass.

```bash
git add docs/project/current-state.md docs/project/llm-handoff.md \
  docs/superpowers/reports/2026-07-22-d04-lightweight-launch-gate-verification.md
git commit -m "docs: verify lightweight D04 launch readiness"
```
