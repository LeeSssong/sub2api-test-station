# refactorUIUXv0.1 Test Station Release Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the independent test-station release controller create a verified pre-release backup, bind candidate image identity, validate JSON readiness, and automatically restore the previous application release on candidate failure.

**Architecture:** Keep the existing local orchestrator and dedicated host executor. Add one host-only backup helper with a narrow contract; extend the executor to resolve the active release, invoke the helper, verify the loaded image, gate success on health/readiness, and re-run the previous Compose release on failure. Preserve the test-station named volumes and never reuse the production blue/green controller.

**Tech Stack:** Bash, Docker Buildx/Compose, SSH/SCP, Python 3 JSON helpers, SHA-256, shell contract tests.

**Spec:** `docs/superpowers/specs/2026-09-20-refactor-uiux-test-station-release-hardening-design.md`

---

## Global constraints

- Work only in `codex/refactor-uiux-release-controller`, based on `origin/main@645ce06834698cebcd6707836a234d7096e0a081`.
- Do not edit `docs/project/project-progress.md`, the task queue, sub business code, migrations, frontend, Compose topology, or Caddy routes.
- Do not contact the test-station host from tests and do not deploy.
- Every Docker Compose command must use `--project-name sub2api-test-station`.
- Never run `down`, `down -v`, `volume rm`, or restore a backup automatically.
- Use TDD for each behavior: add one failing assertion, run it and confirm the intended failure, then add the minimum implementation.

### Task 1: Verified test-station backup helper

**Files:**
- Create: `ops/backup-sub2api-test-station-host.sh`
- Create: `tests/operations/backup_sub2api_test_station_host_test.sh`

- [ ] **Step 1: Write the failing happy-path backup test**

Create a fixture with fake `docker`, `tar`, `sha256sum`, an active Compose/env pair, and output directories. The fake Docker contract must accept only:

```bash
docker compose --project-name sub2api-test-station --env-file "$ACTIVE_ENV" -f "$ACTIVE_COMPOSE" exec -T test-station-postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc'
docker compose --project-name sub2api-test-station --env-file "$ACTIVE_ENV" -f "$ACTIVE_COMPOSE" exec -T test-station-postgres pg_restore --list /tmp/test-station-backup/postgres.dump
docker compose --project-name sub2api-test-station --env-file "$ACTIVE_ENV" -f "$ACTIVE_COMPOSE" exec -T test-station-redis sh -c 'redis-cli --no-auth-warning -a "$REDIS_PASSWORD" SAVE >/dev/null'
docker cp test-station-redis-container:/data/dump.rdb <partial>/redis-dump.rdb
docker cp test-station-api-container:/app/data/. <partial>/app-data/
```

Assert the promoted backup contains exactly `postgres.dump`, `redis-dump.rdb`, `app-data.tar.gz`, `SHA256SUMS`, and `metadata.json`, with directory mode 0700, file mode 0600, valid checksums, and no `.partial-*` directory.

- [ ] **Step 2: Run the backup test and verify RED**

Run:

```bash
bash tests/operations/backup_sub2api_test_station_host_test.sh
```

Expected: FAIL because `ops/backup-sub2api-test-station-host.sh` does not exist.

- [ ] **Step 3: Implement minimal backup helper**

Implement these focused helpers:

```bash
fail(){ printf 'test_station_backup status=failed: %s\n' "$1" >&2; exit 1; }
sha256_file(){ sha256sum "$1" | awk '{print $1}'; }
reject_symlink_components(){ ...; }
canonical_file(){ ...; }
canonical_directory(){ ...; }
```

Required arguments:

```text
--compose <absolute file inside active release>
--env-file <absolute 0600 regular file inside active release>
--deploy-root <absolute directory>
--timestamp <YYYYMMDDTHHMMSSZ>
```

Use `<deploy-root>/backups/.partial-<timestamp>-$$`, promote with `mv` to `<deploy-root>/backups/<timestamp>`, validate all artifacts before promotion, and print only:

```text
test_station_backup status=succeeded backup_dir=<absolute path>
```

- [ ] **Step 4: Run the happy path and verify GREEN**

Run the focused test. Expected: PASS and exactly one promoted backup set.

- [ ] **Step 5: Add one failing test per backup safety behavior**

Add and run RED tests for:

```text
wrong Compose project identity
symlink in deploy/backup/compose/env path
empty PostgreSQL dump
pg_restore validation failure
missing or empty Redis RDB
corrupt app-data tar
checksum validation failure
pre-existing final timestamp directory
unknown historical backup files remain untouched
```

Each failure must leave no promoted timestamp directory and must not alter existing backup sets.

- [ ] **Step 6: Implement the minimum validation and cleanup behavior**

Use an EXIT trap that removes only the current `.partial-*` directory. Reject path/project problems before Docker commands. Generate metadata schema 1:

```json
{"schema_version":1,"created_at":"<timestamp>","project_name":"sub2api-test-station","sha256_verified":true}
```

- [ ] **Step 7: Verify and commit Task 1**

Run:

```bash
bash -n ops/backup-sub2api-test-station-host.sh
bash tests/operations/backup_sub2api_test_station_host_test.sh
git diff --check
```

Commit:

```bash
git add ops/backup-sub2api-test-station-host.sh tests/operations/backup_sub2api_test_station_host_test.sh
git commit -m "ops: add verified test station backup"
```

### Task 2: Active-release resolution, image identity, readiness, and rollback

**Files:**
- Modify: `ops/deploy-sub2api-test-station-host.sh`
- Modify: `tests/operations/deploy_sub2api_test_station_host_test.sh`

- [ ] **Step 1: Replace the shallow fixture with a stateful fake Docker fixture**

The fixture must model:

```text
previous release compose/env and source identity
candidate release compose/env
active API config_files label
candidate and previous image IDs
six service states
candidate failure injection stages
previous release restore invocation
```

Keep existing unsafe path and checksum tests.

- [ ] **Step 2: Add failing tests for pre-switch validation**

Add tests that expect no candidate `up` when:

```text
release-state.json is missing or invalid
state project is not sub2api-test-station
state release_dir differs from the API container config_files release
previous compose/Caddy/.env is missing or a symlink
backup helper fails
loaded candidate tag image ID differs from --image-id
```

Run the host executor test and confirm failures are caused by missing validation.

- [ ] **Step 3: Implement active release and candidate identity validation**

Add required arguments:

```text
--backup-script
--image-id
--migration-set-sha256
```

Parse the active API Compose label using the exact `test-station-api` container filter. Parse state JSON with Python and emit only validated scalar fields. Require state/project/API release agreement. After `docker load`, verify:

```bash
actual_image_id=$($docker_bin image inspect --format '{{.Id}}' "sub2api-test-station-runtime:$source_commit")
[[ "$actual_image_id" == "$image_id" ]] || fail 'loaded image identity mismatch'
```

Invoke the backup helper before candidate `up` and capture its single `backup_dir=` field.

- [ ] **Step 4: Add failing readiness tests**

Add fake probe responses for:

```text
health 200 JSON status=ok
readyz 200 application/json status=ready
readyz 200 text/html
readyz 503 JSON status=not_ready
malformed JSON
success fewer than three consecutive attempts
```

The candidate succeeds only after three consecutive valid health/readiness pairs.

- [ ] **Step 5: Implement probe helpers**

Use a command override `TEST_STATION_PROBE_BIN` for tests and `curl` by default. Capture status, Content-Type, and body separately. Parse JSON with Python. Bound defaults:

```bash
probe_attempts=${TEST_STATION_PROBE_ATTEMPTS:-12}
probe_consecutive=${TEST_STATION_PROBE_CONSECUTIVE:-3}
probe_interval=${TEST_STATION_PROBE_INTERVAL_SECONDS:-5}
```

Require positive integers, with interval allowing `0` only in `TEST_STATION_TEST_MODE=true`.

- [ ] **Step 6: Add failing rollback tests**

Inject failures at candidate Compose start, worker health, Caddy health, and JSON readiness. Assert each case:

```text
invokes candidate up at most once
invokes previous release up after candidate mutation began
never invokes down or volume removal
re-validates previous six-service state and probes
leaves the previous success release-state unchanged
writes candidate failure.json with rolled_back=true when restore succeeds
returns nonzero even when automatic restore succeeds
```

Also test restore failure: return nonzero, preserve candidate/backup, and record `rolled_back=false`.

- [ ] **Step 7: Implement rollback trap and state schema v2**

Track `candidate_started=false`. On failures after setting it true, call `restore_previous`. Do not restore on pre-switch validation failures.

On success atomically write:

```json
{
  "schema_version":2,
  "source_commit":"...",
  "source_tree":"...",
  "migration_set_sha256":"...",
  "image_archive_sha256":"...",
  "image_id":"sha256:...",
  "image_tag":"sub2api-test-station-runtime:<commit>",
  "release_dir":"...",
  "previous_release_dir":"...",
  "backup_dir":"...",
  "project_name":"sub2api-test-station",
  "result":"succeeded",
  "rolled_back":false,
  "updated_at":"..."
}
```

Write candidate `failure.json` through a temporary file and `mv`; include a controlled stage enum rather than raw stderr.

- [ ] **Step 8: Verify and commit Task 2**

Run:

```bash
bash -n ops/deploy-sub2api-test-station-host.sh
bash tests/operations/deploy_sub2api_test_station_host_test.sh
bash tests/operations/backup_sub2api_test_station_host_test.sh
git diff --check
```

Commit:

```bash
git add ops/deploy-sub2api-test-station-host.sh tests/operations/deploy_sub2api_test_station_host_test.sh
git commit -m "ops: rollback failed test station releases"
```

### Task 3: Local provenance bundle and metadata

**Files:**
- Modify: `ops/release-sub2api-test-station.sh`
- Modify: `tests/operations/release_sub2api_test_station_contract_test.sh`

- [ ] **Step 1: Build an executable fake-tool contract test**

Replace grep-only assertions with a temporary Git repository and fake `git`, `docker`, `ssh`, and `scp` commands. Retain syntax checks. Assert no Docker/SSH call for non-main, dirty tree, origin drift, unsafe target, or missing source files.

- [ ] **Step 2: Add failing candidate metadata tests**

On the happy path require the script to:

```text
inspect the locally built candidate tag for a sha256 image ID
compute a 64-hex migration-set SHA over sorted backend/migrations SQL content
copy backup-sub2api-test-station-host.sh into the bundle
pass --backup-script, --image-id, and --migration-set-sha256 to the host executor
```

Run the test and verify RED against the existing orchestrator.

- [ ] **Step 3: Implement metadata generation and transfer**

Use the existing normalized migration-set algorithm from repository tooling; do not modify migrations. Validate both hashes before contacting SSH. Add the backup helper to `scp`. Keep fixed target and deploy root.

- [ ] **Step 4: Add failure tests for malformed image ID and migration hash**

The script must fail before staging/SSH if the local image inspect result is not `sha256:<64 hex>` or migration hash generation fails.

- [ ] **Step 5: Verify and commit Task 3**

Run:

```bash
bash -n ops/release-sub2api-test-station.sh
bash tests/operations/release_sub2api_test_station_contract_test.sh
bash tests/operations/deploy_sub2api_test_station_host_test.sh
bash tests/operations/backup_sub2api_test_station_host_test.sh
git diff --check
```

Commit:

```bash
git add ops/release-sub2api-test-station.sh tests/operations/release_sub2api_test_station_contract_test.sh
git commit -m "ops: bind test station release metadata"
```

### Task 4: Runbook and candidate verification

**Files:**
- Modify: `docs/operations/independent-test-station-handoff.md`
- Modify: `docs/superpowers/specs/2026-09-04-independent-test-station-release-controller-design.md`

- [ ] **Step 1: Correct the historical design discrepancy**

Mark the 2026-09-04 design as superseded for rollback details and link to the 2026-09-20 specification. Do not rewrite historical facts silently.

- [ ] **Step 2: Update the operations handoff**

Document schema v2 fields, backup path/content, automatic application rollback, named-volume preservation, JSON readiness gate, failure evidence, and the rule that current HTML `/readyz` prevents deployment until the application fix lands.

- [ ] **Step 3: Run final focused verification**

Run:

```bash
bash -n ops/backup-sub2api-test-station-host.sh
bash -n ops/deploy-sub2api-test-station-host.sh
bash -n ops/release-sub2api-test-station.sh
bash tests/operations/backup_sub2api_test_station_host_test.sh
bash tests/operations/deploy_sub2api_test_station_host_test.sh
bash tests/operations/release_sub2api_test_station_contract_test.sh
git diff --check
git status --short
```

Expected: all tests PASS; only task files differ before the final commit.

- [ ] **Step 4: Commit documentation and report candidate**

```bash
git add docs/operations/independent-test-station-handoff.md \
  docs/superpowers/specs/2026-09-04-independent-test-station-release-controller-design.md
git commit -m "docs: document test station rollback gate"
```

Report branch, baseline, commits, files, tests, no migration/config/business changes, `downtime_required=unverified until root preflight`, and status `READY_FOR_ROOT_REVIEW`. Do not merge, push, deploy, or remove the worktree.
