# 飞书生产 dry-run 激活 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Production writes in Tasks 2-5 require the user's action-time reply `确认执行临时灾备和飞书配置`; Task 6 (`enabled`) is deliberately outside this plan and requires a later, separate approval.

**Goal:** 为 `GPT-Pro` 和 `GPT-Plus` 建立不超过 Neko 已验证并发的临时主/灾备路由，安全安装飞书回调配置，并在真实群聊完成 `disabled -> dry_run` 验收且证明 Sub2API 零路由写入。

**Architecture:** Sub2API 仍是分组、账号和调度的唯一权威来源；灾备准备只通过 `v0.1.161` 原生 Admin API 进行受限账号写入，不直接写 Sub2API PostgreSQL。飞书回调只公开精确 `POST /relay-ops/api/feishu/events`；`relay-ops` 先在 `disabled` 验证协议与回复，再在 `dry_run` 执行完整预检和审计，但不调用任何路由写 API。

**Tech Stack:** Sub2API `v0.1.161` / commit `19149ca196eeae4a4482e5299dc6fa4ba0b06c8c`，Go 1.24.13 `relay-ops-service`，PostgreSQL，Docker Compose，Caddy，飞书 OpenAPI v3 和 `im.message.receive_v1`。

## Global Constraints

- 本计划开始前必须收到用户当次明确回复 `确认执行临时灾备和飞书配置`；`继续`、`同意`或旧会话授权不替代这个生产写入门禁。
- 本计划的终点是 `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`。不得设置 `enabled`，不得真实切换公开分组。
- 不读取、打印或返回生产 `.env`、Admin API Key、上游 Key、App Secret、verification token、Encrypt Key、Cookie、JWT 或密码。
- 秘密只通过剪贴板到 SSH 标准输入或服务器本地文件转移，命令行参数、shell 历史、Git、日志和审计表中不出现原值。
- 只能用 Sub2API 原生 Admin API 写账号，不直接写 Sub2API PostgreSQL，不发送 `confirm_mixed_channel_risk`。
- 不修改两个公开分组的名称、`1.0x` 用户倍率、公开状态、模型定价、用户、用户 Key、余额、支付或注册。
- 不修改 Wawazz 主账号 ID `8`。Wawazz `INSUFFICIENT_BALANCE` 保持为独立运营阻塞，不通过飞书配置伪装恢复。
- 不重建 Sub2API、PostgreSQL、Redis 或 Caddy；模式变更只允许重建 `relay-ops`。
- 任一分组出现 `mixed`、`none` 或 `partial`，任一秘密泄漏，任一账号模型不全，或任一基础容器 ID 变化时立即停止，将命令模式保持/退回 `disabled`。

---

## File And State Map

- Local source of truth: `docs/superpowers/specs/2026-07-20-feishu-deterministic-command-control-design.md`
- Local runbook: `docs/runbooks/feishu-command-control.md`
- Existing production evidence: `docs/superpowers/reports/2026-07-20-feishu-command-disabled-production-verification.md`
- New production evidence: `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md`
- Production directory: `/opt/sub2api/production`
- Production secret directory: `/opt/sub2api/secrets`
- Production evidence directory: `/opt/sub2api/production/evidence/feishu-dry-run-20260720`
- Runtime routing file: `/opt/sub2api/secrets/feishu-routing.json`
- Fixed current objects: `GPT-Pro` group ID `2`, `GPT-Plus` group ID `6`, Neko primary account ID `7`, Wawazz primary account ID `8`, Aliu Pro backup account ID `2`
- Runtime-created object: the account ID returned by idempotent duplication of Neko account `7`; record it as `NEKO_PLUS_BACKUP_ID` in the redacted evidence, never infer it from list order
- Required public models for both routes: `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`, `gpt-5.4`, `gpt-5.4-mini`

---

### Task 1: Reconfirm Authorization And Freeze A Pre-change Baseline

**Files:**
- Read: `docs/project/current-state.md`
- Read: `docs/project/llm-handoff.md`
- Read: `docs/runbooks/feishu-command-control.md`
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/pre-change.json`

**Interfaces:**
- Consumes: production Admin API through the already mounted key, without exposing the key
- Produces: a redacted baseline whose account fields are limited to `id`, `name`, `platform`, `status`, `schedulable`, `group_ids`, `concurrency`, `rate_multiplier`, `credentials_status`, runtime block timestamps and sorted model names

- [x] **Step 1: Check the action-time authorization**

Require the exact current-turn reply:

```text
确认执行临时灾备和飞书配置
```

Expected: the exact reply is present after this plan was shown. Otherwise stop after planning and perform no server or browser writes.

- [x] **Step 2: Re-run the local command-control regression gate**

Run:

```bash
cd /Users/gongtengxinwen/Documents/sub2api搭建/relay-ops-service
go test ./... -race -count=1
go vet ./...
cd /Users/gongtengxinwen/Documents/sub2api搭建
bash tests/relay_ops/validate_relay_ops_contract.sh
```

Expected: all 24 Go packages pass the race run, `go vet ./...` exits `0`, and the contract ends with `PASS: relay-ops container and routing contracts`.

- [x] **Step 3: Capture infrastructure identity and health**

From `/opt/sub2api/production`, record the container IDs for `sub2api`, `postgres`, `redis`, `caddy`, and `relay-ops`, plus relay-ops health/restart count and current modes. Do not record environment values.

Expected: all five containers are running/healthy, relay-ops restart count is `0`, `RELAY_OPS_MODE=read_only`, and `RELAY_OPS_FEISHU_COMMAND_MODE=disabled`.

- [x] **Step 4: Capture the redacted Sub2API baseline**

Read group IDs `2` and `6`, account IDs `2`, `7`, and `8`, and the model lists for those accounts. Write only the allowlisted fields to `pre-change.json` with mode `0600`; write a SHA-256 beside it.

Expected:

```text
GPT-Pro(2): public, active, OpenAI, 1.0x
GPT-Plus(6): public, active, OpenAI, 1.0x
Neko(7): active, schedulable, group_ids=[2], concurrency=3, cost=0.10x, six required models
Wawazz(8): active, schedulable, group_ids=[6], concurrency=1, cost=0.05x, six required models
Aliu(2): active, not schedulable, six required models
```

Stop if any identity, group, status, multiplier, binding, concurrency, credential status or model set differs. Wawazz's known upstream balance warning may remain, but an account-level inactive/error/temporary block is a stop condition because dry-run state would no longer match the approved design.

---

### Task 2: Create The Neko Plus Backup Without Increasing Shared-key Capacity

**Files:**
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/account-preparation.json`
- Modify through Sub2API Admin API: account ID `7`
- Create through Sub2API Admin API: one duplicate of account ID `7`

**Interfaces:**
- Consumes: source Neko account ID `7` and stable idempotency key `feishu-gpt-plus-backup-neko-20260720-v1`
- Produces: `NEKO_PLUS_BACKUP_ID`, an active but unschedulable/unbound account named `neko-production-plus-backup` with concurrency `1`, while Neko primary ID `7` has concurrency `2`

- [x] **Step 1: Duplicate Neko idempotently**

Send exactly:

```http
POST /api/v1/admin/accounts/7/duplicate
Idempotency-Key: feishu-gpt-plus-backup-neko-20260720-v1
```

Expected: HTTP success returns one new account; repeating the same operation key returns the same account ID. Save only that ID and redacted account fields as `NEKO_PLUS_BACKUP_ID`.

- [x] **Step 2: Quarantine and normalize the duplicate**

Send a partial update to the returned account ID:

```json
{
  "name": "neko-production-plus-backup",
  "concurrency": 1,
  "group_ids": []
}
```

Do not include credentials, `rate_multiplier`, `status` or `confirm_mixed_channel_risk` in the payload.

Expected: the duplicate is `active`, `schedulable=false`, `group_ids=[]`, concurrency `1`, cost multiplier `0.10x`, and credential status is present/valid. Its six-model set exactly matches the required model list.

- [x] **Step 3: Reduce Neko primary concurrency**

Send the only update to account ID `7`:

```json
{
  "concurrency": 2
}
```

Expected: ID `7` remains `active`, `schedulable=true`, `group_ids=[2]`, cost multiplier `0.10x`, and has all six models. The duplicate remains unschedulable, so live Neko traffic capacity is temporarily `2`, not `3` or `4`.

- [x] **Step 4: Verify aggregate capacity and rollback readiness**

Record both Neko account states and assert:

```text
primary configured concurrency 2 + backup configured concurrency 1 = validated shared-key ceiling 3
currently schedulable Neko concurrency = 2
```

If any duplicate write is ambiguous, re-read using the stable idempotency key and returned ID; do not issue another duplicate with a new key. If primary concurrency cannot be verified as `2`, restore it to `3`, leave the duplicate unbound and unschedulable, and stop.

---

### Task 3: Preflight Both Backup Routes And Install The Routing Contract

**Files:**
- Create on server: `/opt/sub2api/secrets/feishu-routing.json`
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/routing-preflight.json`

**Interfaces:**
- Consumes: group IDs `2`/`6`, account IDs `7`/`8`/`2`/`NEKO_PLUS_BACKUP_ID`, and the six-model allowlist
- Produces: a strict `0600` routing file loadable by `routingcontrol.LoadConfig`

- [x] **Step 1: Re-read all four route accounts**

Verify the route matrix:

| Public group | Primary | Backup |
|---|---:|---:|
| `GPT-Pro` (`2`) | Neko `7` | Aliu `2` |
| `GPT-Plus` (`6`) | Wawazz `8` | Neko `NEKO_PLUS_BACKUP_ID` |

Expected: all four IDs are unique; both backups are active, not runtime-blocked, credential-valid, and expose all six required models. Aliu stays unschedulable and is not modified. The Neko duplicate stays unbound and unschedulable.

- [x] **Step 2: Create the route file directly on the server**

Write this structure using the actual captured duplicate ID:

```json
{
  "groups": [
    {
      "name": "GPT-Pro",
      "public_group_id": 2,
      "primary_account_id": 7,
      "backup_account_id": 2,
      "required_models": [
        "gpt-5.6-sol",
        "gpt-5.6-terra",
        "gpt-5.6-luna",
        "gpt-5.5",
        "gpt-5.4",
        "gpt-5.4-mini"
      ]
    },
    {
      "name": "GPT-Plus",
      "public_group_id": 6,
      "primary_account_id": 8,
      "backup_account_id": NEKO_PLUS_BACKUP_ID,
      "required_models": [
        "gpt-5.6-sol",
        "gpt-5.6-terra",
        "gpt-5.6-luna",
        "gpt-5.5",
        "gpt-5.4",
        "gpt-5.4-mini"
      ]
    }
  ]
}
```

`NEKO_PLUS_BACKUP_ID` above is a runtime substitution with the numeric ID returned in Task 2, not literal JSON. Install the result as `root:10002`, mode `0640`; do not store it in Git.

- [x] **Step 3: Validate the routing JSON structure before mounting it**

Run on the server without printing the JSON:

```bash
jq -e '
  (.groups | length == 2) and
  ([.groups[].name] | sort == ["GPT-Plus", "GPT-Pro"]) and
  ([.groups[].public_group_id] | unique | length == 2) and
  ([.groups[] | .primary_account_id, .backup_account_id] | unique | length == 4) and
  (all(.groups[]; (.required_models | length == 6) and ((.required_models | unique | length) == 6)))
' /opt/sub2api/secrets/feishu-routing.json >/dev/null
```

Expected: exit `0`. The exact Go `routingcontrol.LoadConfig` parser is intentionally validated later when relay-ops first starts in `dry_run`; `disabled` mode does not load the routing file, and this plan does not start a second worker against the production command queue.

---

### Task 4: Install Feishu Secrets And Complete `disabled` Acceptance

**Files:**
- Create on server: `/opt/sub2api/secrets/feishu-app-id`
- Create on server: `/opt/sub2api/secrets/feishu-app-secret`
- Create on server: `/opt/sub2api/secrets/feishu-verification-token`
- Create on server: `/opt/sub2api/secrets/feishu-encrypt-key`
- Modify on server: `/opt/sub2api/production/.env` by exact key replacement only
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/disabled-acceptance.json`

**Interfaces:**
- Consumes: existing Feishu app `星桥AI监控Agent` (`cli_aad650b7f138dcd1`), its masked App Secret, and server-generated verification/encryption values
- Produces: an active callback and reply client while `RELAY_OPS_FEISHU_COMMAND_MODE=disabled`

- [x] **Step 1: Generate callback secrets on the production server**

Generate the verification token and Encrypt Key directly into their final files, never through terminal output:

```bash
sudo install -d -m 0750 -o root -g 10002 /opt/sub2api/secrets
printf '%s' 'cli_aad650b7f138dcd1' | sudo install -m 0640 -o root -g 10002 /dev/stdin /opt/sub2api/secrets/feishu-app-id
openssl rand -hex 32 | tr -d '\n' | sudo install -m 0640 -o root -g 10002 /dev/stdin /opt/sub2api/secrets/feishu-verification-token
openssl rand -base64 48 | tr -d '\n' | sudo install -m 0640 -o root -g 10002 /dev/stdin /opt/sub2api/secrets/feishu-encrypt-key
```

Transfer the masked App Secret by clicking Copy in the logged-in Feishu UI and piping the local clipboard directly to the remote file's standard input through the established SSH control channel; clear the clipboard immediately after the remote file size/permission check. Install the App Secret as `root:10002`, mode `0640`.

Expected: all five files are regular, nonempty, mode `0640`, owned by `root:10002`; checks report only names, modes and byte counts, never hashes or contents.

- [x] **Step 2: Configure the five host/container references while staying disabled**

Set these exact nonsecret references in `/opt/sub2api/production/.env` without displaying the file:

```text
RELAY_OPS_FEISHU_COMMAND_MODE=disabled
RELAY_OPS_FEISHU_APP_ID_HOST_FILE=/opt/sub2api/secrets/feishu-app-id
RELAY_OPS_FEISHU_APP_SECRET_HOST_FILE=/opt/sub2api/secrets/feishu-app-secret
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_HOST_FILE=/opt/sub2api/secrets/feishu-verification-token
RELAY_OPS_FEISHU_ENCRYPT_KEY_HOST_FILE=/opt/sub2api/secrets/feishu-encrypt-key
RELAY_OPS_FEISHU_ROUTING_HOST_FILE=/opt/sub2api/secrets/feishu-routing.json
RELAY_OPS_FEISHU_APP_ID_FILE=/run/secrets/feishu-app-id
RELAY_OPS_FEISHU_APP_SECRET_FILE=/run/secrets/feishu-app-secret
RELAY_OPS_FEISHU_VERIFICATION_TOKEN_FILE=/run/secrets/feishu-verification-token
RELAY_OPS_FEISHU_ENCRYPT_KEY_FILE=/run/secrets/feishu-encrypt-key
RELAY_OPS_FEISHU_ROUTING_FILE=/run/secrets/feishu-routing.json
```

Back up `.env` as a mode-`0600` root-readable file before replacement. Validate Compose with `config --quiet`; do not run a command that renders resolved environment values.

- [x] **Step 3: Recreate only relay-ops in disabled mode**

Run:

```bash
cd /opt/sub2api/production
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops
docker compose --env-file .env -f compose.yaml ps relay-ops
```

Expected: relay-ops becomes healthy with restart count `0`; `/healthz`, `/readyz`, `/pricing`, and `/ops` remain healthy; Sub2API, PostgreSQL, Redis and Caddy IDs are unchanged. Container inspection shows five read-only secret mounts and the five container paths, but no secret values.

- [x] **Step 4: Configure Feishu event subscription**

In the already logged-in Feishu developer console:

1. Keep the existing permissions `im:message.group_at_msg:readonly` and `im:message:send_as_bot`.
2. Resolve the active HTTPS origin from the production `SITE_ADDRESS` without printing the environment file, then configure that origin plus `/relay-ops/api/feishu/events` as the callback URL. Do not persist the temporary hostname in Git or ordinary documentation.
3. Transfer the server-generated verification token and Encrypt Key to the UI via clipboard without displaying them.
4. Subscribe only to `im.message.receive_v1`.
5. Publish the minimal app version required for the event subscription.

Expected: challenge succeeds, event subscription is configured, and no broader contact, file, private-chat or tenant permissions are added.

- [x] **Step 5: Verify disabled behavior in one real group**

Add the bot to the designated test group and send:

```text
查询当前分组状态
```

Prepend the actual bot mention rendered by Feishu; the normalized command after the structured leading mention is removed must equal `查询当前分组状态`.

Expected reply contains `命令功能未启用`、`rejected` and a short audit ID. Query the relay-ops audit table and confirm one `command_disabled` record. Compare the Sub2API account snapshot with the post-Task-3 baseline and prove `group_ids` and `schedulable` are unchanged.

---

### Task 5: Transition To `dry_run` And Verify All Five Commands

**Files:**
- Modify on server: `/opt/sub2api/production/.env` (`RELAY_OPS_FEISHU_COMMAND_MODE` only)
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/dry-run-before.json`
- Create on server: `/opt/sub2api/production/evidence/feishu-dry-run-20260720/dry-run-after.json`
- Create locally: `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md`

**Interfaces:**
- Consumes: healthy disabled callback, valid routing file and the four prepared route accounts
- Produces: five real Feishu command audit records and byte-for-byte-equivalent redacted routing state before/after

- [x] **Step 1: Freeze the dry-run before snapshot**

Capture account IDs `2`, `7`, `8`, and `NEKO_PLUS_BACKUP_ID` plus groups `2` and `6` using the same allowlist and canonical JSON ordering as Task 1. Save mode `0600` and record SHA-256.

Expected current role for both groups: `primary`. Neko primary concurrency is `2`; Neko Plus backup concurrency is `1`, unbound and unschedulable; Aliu is unschedulable.

- [x] **Step 2: Change only the command mode and recreate relay-ops**

Set:

```text
RELAY_OPS_FEISHU_COMMAND_MODE=dry_run
```

Then run:

```bash
cd /opt/sub2api/production
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --no-deps --force-recreate relay-ops
docker compose --env-file .env -f compose.yaml ps relay-ops
```

Expected: only relay-ops ID changes; it is healthy with restart count `0`; all four base services keep their IDs.

The first `dry_run` start is also the exact production validation of `routingcontrol.LoadConfig`. A routing parse/configuration error must leave relay-ops unhealthy; immediately restore `disabled`, recreate only relay-ops and stop before sending any command.

- [x] **Step 3: Send all five exact commands**

Send each as a separate group message with only the Feishu-rendered leading bot mention:

```text
切换 GPT-Pro 到灾备
切换 GPT-Plus 到灾备
恢复 GPT-Pro 主分组
恢复 GPT-Plus 主分组
查询当前分组状态
```

Expected:

- The two `切换 ... 到灾备` replies are `succeeded` dry-run projections from `primary` to `backup`.
- The two `恢复 ... 主分组` replies are `no_op`, because dry-run never changed the real current role from `primary`.
- The query reply reports both groups as `primary`.
- Every reply contains an audit ID and no account credentials, secret, raw HTTP response, server path, full `chat_id` or full `open_id`.

- [x] **Step 4: Verify rejection boundaries without creating writes**

Send one unknown command with an extra argument and one private-chat command. Do not use bot/app/system impersonation in production; retain automated test evidence for those sender types.

Expected: the unknown group command returns the fixed help response once; private chat is ignored/rejected according to the design; neither produces a routing operation. Database uniqueness on `event_id` remains active, and any natural Feishu retry maps to the existing event rather than a second execution.

- [x] **Step 5: Prove zero Sub2API route writes**

Capture `dry-run-after.json` with the identical serializer and compare it with `dry-run-before.json`.

Expected exact equality for:

```text
group names, platform, status, public flag and 1.0x multipliers
all four account IDs, names, status, schedulable and group_ids
both Neko account concurrency values (2 and 1)
all four credential-status indicators and six-model sets
```

The two files' canonical SHA-256 values must match. A mismatch is a failed acceptance: immediately return mode to `disabled`, do not attempt an automatic reverse write, and inspect the authoritative Sub2API state manually.

- [x] **Step 6: Verify audit, health and public paths**

Query only `relay_ops.feishu_command_events` allowlisted columns and confirm five accepted command records plus the deliberate rejection tests. Verify `/healthz`, `/readyz`, `/pricing`, `/ops`, Caddy exact route, relay-ops logs and restart count.

Expected: no `partial`, `mixed`, `none`, unknown error code, reply exhaustion or secret-bearing field. Neko production monitor remains healthy; Wawazz may remain `DEGRADED` only for the already known insufficient-balance reason.

- [x] **Step 7: Write the production verification report and update handoff truth**

Create `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md` with:

- image tag/ID and container identity comparison;
- redacted route matrix and `NEKO_PLUS_BACKUP_ID`;
- five command outcomes and rejection outcomes;
- before/after canonical hash equality;
- audit counts and health checks;
- explicit statement that no real route switch occurred;
- explicit remaining gate: `enabled` requires a new user approval.

Update `docs/project/current-state.md` and `docs/project/llm-handoff.md` to point to the report and state that production is stopped at `dry_run`.

---

### Task 6: Stop And Request Separate `enabled` Approval

**Files:**
- Read: `docs/superpowers/reports/2026-07-20-feishu-production-dry-run-verification.md`
- No production file or Sub2API mutation in this task

**Interfaces:**
- Consumes: completed dry-run report
- Produces: a user decision request, not an implementation action

- [x] **Step 1: Present the dry-run evidence**

Report the route matrix, all five results, zero-write proof, health, known Wawazz balance issue, and any residual risk.

- [x] **Step 2: Stop at `dry_run`**

Do not switch to `enabled`, do not send a real failover command, and do not perform sync/SSE traffic through a backup account.

Expected: the next production action is blocked on a new, explicit approval that names the target group and authorizes `enabled` acceptance.

---

## Rollback And Stop Conditions

### Account preparation rollback

- If duplication is ambiguous, replay only the same idempotency key and recover the committed duplicate ID; never use a second key.
- If the duplicate is accidentally bound or schedulable, set that exact returned account to `schedulable=false`, then set `group_ids=[]`, and re-read. Do not delete it during incident handling.
- If Neko primary cannot remain healthy at concurrency `2`, restore account ID `7` to `concurrency=3`, keep the duplicate unbound/unschedulable, return command mode to `disabled`, and stop.
- Aliu and Wawazz are never modified by rollback in this plan.

### Feishu rollback

- Set `RELAY_OPS_FEISHU_COMMAND_MODE=disabled`, validate Compose quietly, and recreate only relay-ops.
- Disable/remove the event subscription in Feishu if callback verification or secret handling is suspect.
- Preserve the command audit table and evidence files. Do not delete records to make a failed acceptance appear clean.
- Secret rotation replaces the affected server file atomically and updates the Feishu console; values are never copied into chat or a report.

### Mandatory stop conditions

- Any account ID/name/group/model mismatch.
- Any account credential status invalid, inactive status or temporary runtime block.
- Any HTTP 409 `mixed_channel_warning`; never retry with `confirm_mixed_channel_risk`.
- Any dry-run snapshot difference in `group_ids`, `schedulable`, concurrency or group multiplier.
- Any `partial`, `mixed`, `none`, unknown audit value, secret leak or full identifier in reply/log/audit.
- Any recreation or ID change of Sub2API, PostgreSQL, Redis or Caddy.
- Any request to enter `enabled` before the dry-run report is complete and separately approved.

## Final Acceptance

- [x] One idempotent Neko duplicate exists as the unbound/unschedulable `GPT-Plus` backup with concurrency `1`.
- [x] Neko primary ID `7` remains the healthy `GPT-Pro` primary with concurrency `2`; shared-key configured concurrency remains `3`.
- [x] Aliu ID `2` is the untouched unschedulable `GPT-Pro` backup; Wawazz ID `8` is the untouched `GPT-Plus` primary.
- [x] Feishu challenge and one disabled command pass before the mode transition.
- [x] All five commands pass their expected `dry_run`/`no_op`/query behavior in a real group.
- [x] Canonical before/after evidence proves no change to Sub2API `group_ids`, `schedulable`, concurrency or public group pricing.
- [x] relay-ops remains healthy; Sub2API, PostgreSQL, Redis and Caddy are not rebuilt.
- [x] The production verification report and handoff are current.
- [x] Production remains at `dry_run`; `enabled` has not been approved or entered.
