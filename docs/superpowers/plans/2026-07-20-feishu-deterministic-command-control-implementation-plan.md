# 飞书确定性运维命令 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 Go `relay-ops-service` 内实现可验证、可去重、可审计的飞书群命令，用 Sub2API `v0.1.161` 原生 Admin API 在 `GPT-Pro` 和 `GPT-Plus` 的主、灾备账号间切换分组绑定。

**Architecture:** 公开 HTTP 入口只验证飞书回调并将结构化事件持久化，后台 worker 领取命令后调用固定动作注册表。路由控制器对 Sub2API 执行读-加目标-复读-移除源-复读，并通过飞书 OpenAPI 把脱敏结果发回原群聊。

**Tech Stack:** Go 1.24.13，`net/http`，`crypto/aes`，`crypto/sha256`，PostgreSQL/pgx v5，Docker Compose，Caddy，Sub2API Admin API v0.1.161，飞书 OpenAPI v3。

## Global Constraints

- 只允许五条已确认的中文命令逐字匹配，不得引入 LLM、模糊匹配、动态参数、shell 或通用管理 API 代理。
- 任意已加入机器人的群聊内，所有真人成员都可执行；私聊、bot、app 和 system 发件人不得执行生产写。
- 只改变公开分组的账号 `group_ids` 绑定和必要时目标账号的 `schedulable=true`；不改用户、API Key、分组名、价格或模型范围。
- 固定 Sub2API 版本契约为 `v0.1.161` / commit `19149ca196eeae4a4482e5299dc6fa4ba0b06c8c`。
- 只允许 `disabled|dry_run|enabled`，默认 `disabled`，首次生产启用顺序必须为 `disabled -> dry_run -> enabled`。
- 凭据、verification token、Encrypt Key 和路由标识只从 `0600` 或 `0640` 文件读取，不输出到 Git、日志、错误或审计 JSON。
- 严格 TDD：每个生产行为都必须先有一个按预期原因失败的测试，再写最小实现并观察其通过。

---

## File Map

- Create `relay-ops-service/internal/feishuevents/events.go`: 回调签名、AES-CBC 解密、challenge 与 v2 事件解析。
- Create `relay-ops-service/internal/feishuevents/events_test.go`: 飞书协议和输入限制测试。
- Create `relay-ops-service/internal/commands/commands.go`: 命令注册表、精确匹配、规范化和核心记录类型。
- Create `relay-ops-service/internal/commands/commands_test.go`: 五条命令、mention、群聊与 sender 边界测试。
- Create `relay-ops-service/internal/commands/worker.go`: 命令领取、模式门、路由执行、结果持久化和回复重试。
- Create `relay-ops-service/internal/commands/worker_test.go`: 去重、租约恢复、三次回复和模式测试。
- Create `relay-ops-service/internal/routingcontrol/controller.go`: 路由配置、状态分类和先目标后源的切换算法。
- Create `relay-ops-service/internal/routingcontrol/controller_test.go`: 成功、no-op、预检失败、写失败、partial 与 dry-run 测试。
- Create `relay-ops-service/internal/feishuapi/client.go`: tenant token 缓存与群聊文本发送。
- Create `relay-ops-service/internal/feishuapi/client_test.go`: fake OpenAPI 契约、缓存、大小限制与脱敏错误测试。
- Modify `relay-ops-service/internal/sub2api/client.go`, `types.go`, `client_test.go`: 增加受限账号/分组/模型读取与两个固定写 API。
- Modify `relay-ops-service/internal/config/config.go`, `config_test.go`: 加载模式、四个飞书秘密文件和路由 JSON。
- Modify `relay-ops-service/internal/store/migrations/001_init.sql`, `postgres.go`, `postgres_test.go`: 命令队列、幂等、租约、分组咨询锁和回复审计。
- Modify `relay-ops-service/internal/http/server.go`, `server_test.go`: 公开精确回调路由，其余 API 继续管理员鉴权。
- Modify `relay-ops-service/internal/app/app.go`, `app_test.go`, `cmd/relay-ops/main.go`: 组装依赖并启动可停止 worker。
- Modify `infra/compose.yaml`, `infra/Caddyfile`, `tests/relay_ops/validate_relay_ops_contract.sh`, `infra/.env.example`: 默认 disabled、只读秘密挂载和精确公开路由。
- Create `config/operations/feishu-routing.example.json`: 无真实 ID 的结构化配置示例。
- Create `docs/runbooks/feishu-command-control.md`: 飞书权限、回调、disabled/dry-run/enabled 验收与回滚步骤。

### Task 1: 固定 Sub2API v0.1.161 受限客户端契约

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`

**Interfaces:**
- Produces: `GetGroup(context.Context, int64) (Group, error)`
- Produces: `GetAccount(context.Context, int64) (Account, error)`
- Produces: `GetAccountModels(context.Context, int64) ([]Model, error)`
- Produces: `SetAccountGroups(context.Context, int64, []int64) (Account, error)`
- Produces: `SetAccountSchedulable(context.Context, int64, bool) (Account, error)`

- [ ] **Step 1: Write the failing native API contract tests**

Add table-driven `httptest.Server` assertions proving exact methods, paths, `x-api-key`, JSON bodies, response envelopes, 2 MiB limits and HTTP 409 classification. The write body assertions must decode into `map[string]any` and assert it contains exactly one key: `group_ids` or `schedulable`.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run:
```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 go test ./internal/sub2api -run 'TestReaderAccount|TestReaderGroup|TestReaderModels' -count=1
```
Expected: compile failure because the five methods and `Account`/`Model` types do not exist.

- [ ] **Step 3: Implement the minimal constrained client methods**

Define:
```go
type Controller interface {
	GetGroup(context.Context, int64) (Group, error)
	GetAccount(context.Context, int64) (Account, error)
	GetAccountModels(context.Context, int64) ([]Model, error)
	SetAccountGroups(context.Context, int64, []int64) (Account, error)
	SetAccountSchedulable(context.Context, int64, bool) (Account, error)
}

type Account struct {
	ID                int64           `json:"id"`
	Name              string          `json:"name"`
	Platform          string          `json:"platform"`
	Status            string          `json:"status"`
	Schedulable       bool            `json:"schedulable"`
	GroupIDs          []int64         `json:"group_ids"`
	CredentialsStatus map[string]bool `json:"credentials_status"`
}

type Model struct { ID string `json:"id"` }
```
Use the existing `do` response limit and envelope decoder. Add a private `jsonRequest` helper that only accepts preconstructed method/path/body values and never exposes arbitrary paths publicly.

- [ ] **Step 4: Run focused and package tests GREEN**

Run the command from Step 2, then `go test ./internal/sub2api -count=1` in the same pinned container. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/sub2api/client.go relay-ops-service/internal/sub2api/types.go relay-ops-service/internal/sub2api/client_test.go
git commit -m "feat: add constrained Sub2API routing client"
```

### Task 2: 加载并校验飞书命令配置

**Files:**
- Modify: `relay-ops-service/internal/config/config.go`
- Modify: `relay-ops-service/internal/config/config_test.go`
- Create: `config/operations/feishu-routing.example.json`

**Interfaces:**
- Produces: `FeishuCommandMode string`
- Produces: `FeishuAppIDFile`, `FeishuAppSecretFile`, `FeishuVerificationTokenFile`, `FeishuEncryptKeyFile`, `FeishuRoutingFile string`
- Produces: `routingcontrol.LoadConfig(path string) (routingcontrol.Config, error)` in Task 5.

- [ ] **Step 1: Write failing mode and secret-file tests**

Test default `disabled`, rejection of any other value, all-or-none secret paths, `dry_run`/`enabled` requiring all five files, and rejection of mode bits other than `0600`/`0640`. Ensure error strings contain labels but never file content.

- [ ] **Step 2: Confirm RED**

Run:
```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 go test ./internal/config -run Feishu -count=1
```
Expected: compile failure for missing config fields/constants.

- [ ] **Step 3: Implement config validation**

Add constants `FeishuCommandDisabled`, `FeishuCommandDryRun`, `FeishuCommandEnabled`. In disabled mode accept either no Feishu files or a complete set; in dry-run/enabled require the complete set. Continue using `validateSecretFile` and never read values in `config.Load`.

Create the example JSON with explicit non-production sentinel IDs and required model sets:
```json
{
  "groups": [
    {"name":"GPT-Pro","public_group_id":900001,"primary_account_id":910001,"backup_account_id":910002,"required_models":["gpt-5.6-sol"]},
    {"name":"GPT-Plus","public_group_id":900002,"primary_account_id":920001,"backup_account_id":920002,"required_models":["gpt-5.6-terra"]}
  ]
}
```

- [ ] **Step 4: Run config tests GREEN**

Run `go test ./internal/config -count=1` in the pinned container. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/config/config.go relay-ops-service/internal/config/config_test.go config/operations/feishu-routing.example.json
git commit -m "feat: validate Feishu command configuration"
```

### Task 3: 实现飞书回调验证与结构化事件解析

**Files:**
- Create: `relay-ops-service/internal/feishuevents/events.go`
- Create: `relay-ops-service/internal/feishuevents/events_test.go`

**Interfaces:**
- Produces: `NewVerifier(verificationToken, encryptKey string, now func() time.Time) (*Verifier, error)`
- Produces: `(*Verifier).Decode(*http.Request, int64) (Envelope, error)`
- Produces: `Envelope{Challenge string; Event *MessageEvent}`
- Produces: stable errors `ErrUnauthorized`, `ErrExpired`, `ErrTooLarge`, `ErrMalformed`.

- [ ] **Step 1: Write failing protocol tests**

Create deterministic helpers that encrypt PKCS#7-padded JSON with AES-256-CBC using `sha256(encryptKey)` and prepend a fixed 16-byte IV. Cover valid encrypted challenge, valid `im.message.receive_v1`, invalid signature/token/ciphertext, timestamp older than five minutes, non-numeric timestamp, 256 KiB body limit, and 4 KiB decoded text limit.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/feishuevents -count=1` in the pinned container. Expected: package missing.

- [ ] **Step 3: Implement the minimal verifier**

Calculate `hex(sha256(timestamp + nonce + encryptKey + rawBody))` with constant-time comparison, reject timestamps outside `300s`, base64-decode `encrypt`, split IV/ciphertext, AES-CBC decrypt, verify every PKCS#7 padding byte, then decode with `json.Decoder.DisallowUnknownFields` only for the outer encrypted wrapper. Validate header token after decrypting. Parse only documented event fields and reject identifiers over 256 bytes.

- [ ] **Step 4: Run package tests GREEN**

Run `go test ./internal/feishuevents -count=1`. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/feishuevents
git commit -m "feat: verify encrypted Feishu callbacks"
```

### Task 4: 实现确定性命令注册表与授权边界

**Files:**
- Create: `relay-ops-service/internal/commands/commands.go`
- Create: `relay-ops-service/internal/commands/commands_test.go`

**Interfaces:**
- Consumes: `feishuevents.MessageEvent`
- Produces: `Parse(event feishuevents.MessageEvent) Decision`
- Produces: `Action{Kind ActionSwitch|ActionStatus; GroupName string; TargetRole routingcontrol.Role}`
- Produces: `Decision{Accepted bool; Ignore bool; Command string; Action Action; ErrorCode string}`.

- [ ] **Step 1: Write failing parser table tests**

Cover all five exact commands, leading/trailing Unicode whitespace, one leading structured mention, unknown text, extra arguments, two commands, URL/code block, private chat, non-user sender, non-text type, missing IDs, and arbitrary group IDs. Assert every valid group user is accepted and no chat whitelist exists.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/commands -run Parse -count=1`. Expected: package or symbols missing.

- [ ] **Step 3: Implement fixed map lookup**

Decode `message.content` as exactly `{"text":"..."}`; remove only a mention key that begins the text and exists as the first structured mention; call `strings.TrimSpace`; look up the result in a package-level immutable map of exactly five entries. Never accept caller-supplied actions.

- [ ] **Step 4: Run parser tests GREEN**

Run `go test ./internal/commands -run Parse -count=1`. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/commands/commands.go relay-ops-service/internal/commands/commands_test.go
git commit -m "feat: add deterministic Feishu command registry"
```

### Task 5: 实现 Sub2API 路由状态与切换算法

**Files:**
- Create: `relay-ops-service/internal/routingcontrol/controller.go`
- Create: `relay-ops-service/internal/routingcontrol/controller_test.go`

**Interfaces:**
- Consumes: `sub2api.Controller`
- Produces: `Role` constants `primary`, `backup`
- Produces: `LoadConfig(path string) (Config, error)`
- Produces: `ReadAll(context.Context) ([]GroupState, error)`
- Produces: `Switch(context.Context, string, Role, bool) Result`
- Produces: result states `succeeded`, `no_op`, `partial`, `failed`, `rejected`, with stable error codes.

- [ ] **Step 1: Write failing configuration tests**

Test exactly two entries named `GPT-Pro` and `GPT-Plus`, positive unique group IDs, distinct main/backup IDs, no account reused across groups, nonempty deduplicated `required_models`, secure file mode, and trailing JSON rejection.

- [ ] **Step 2: Confirm RED for configuration**

Run `go test ./internal/routingcontrol -run Config -count=1`. Expected: package missing.

- [ ] **Step 3: Implement minimal strict config loader**

Use `json.Decoder.DisallowUnknownFields`, require EOF after one value, sort model lists, and return errors that contain group names but not file contents.

- [ ] **Step 4: Write failing state and switch tests**

Use a stateful fake implementing `sub2api.Controller`. Assert: preflight reads group/accounts/models; already-target is no-op with zero writes; dry-run has zero writes; success writes target schedulable only when false, then target complete group IDs, rereads, source complete group IDs, and rereads; unrelated bindings remain; target failure leaves source untouched; source failure returns partial; reread ambiguity returns partial; inactive or missing-model target is rejected; mixed/none state is reported explicitly.

- [ ] **Step 5: Confirm RED for switching**

Run `go test ./internal/routingcontrol -run 'Switch|ReadAll' -count=1`. Expected: missing methods.

- [ ] **Step 6: Implement minimal switch protocol**

Classify a route only when exactly one configured account is currently eligible for the configured group. Before any write compare group ID/name/platform and required model sets, then preserve and sort the account's entire existing `group_ids` on each `PUT`. Never send `confirm_mixed_channel_risk`, never disable the source globally, and never perform an automatic rollback after an uncertain write.

- [ ] **Step 7: Run routing tests GREEN**

Run `go test ./internal/routingcontrol -count=1`. Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add relay-ops-service/internal/routingcontrol
git commit -m "feat: add native Sub2API failover controller"
```

### Task 6: 实现 PostgreSQL 幂等队列与审计

**Files:**
- Modify: `relay-ops-service/internal/store/migrations/001_init.sql`
- Modify: `relay-ops-service/internal/store/postgres.go`
- Modify: `relay-ops-service/internal/store/postgres_test.go`

**Interfaces:**
- Consumes/produces: `commands.Record`
- Produces: `InsertFeishuEvent(context.Context, commands.Record) (bool, error)`
- Produces: `ClaimFeishuCommand(context.Context, time.Time, time.Duration) (*commands.Record, error)`
- Produces: `CompleteFeishuCommand(context.Context, commands.Completion) error`
- Produces: `RecordFeishuReply(context.Context, eventID, messageID string, delivered bool, errorCode string) error`
- Produces: `WithFeishuGroupLock(context.Context, int64, func(context.Context) commands.Completion) (commands.Completion, error)`.

- [ ] **Step 1: Write failing integration tests**

Using existing `openTestStore`, test duplicate `event_id` returns `inserted=false`, two concurrent claims cannot return the same row, an expired lease can be reclaimed, completion snapshots contain only non-sensitive route state, reply attempts increment, and advisory lock serializes the same group while allowing a different group.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/store -run Feishu -count=1`. Expected: missing schema/methods.

- [ ] **Step 3: Add the append/update schema**

Create `relay_ops.feishu_command_events` with `event_id text PRIMARY KEY`, bounded identifiers, nullable whitelist command/action fields, checked status/role/error fields, `before_state`/`after_state jsonb`, `lease_expires_at`, `reply_attempts`, timestamps, and indexes on `(status, received_at)` and `lease_expires_at`. Use `INSERT ... ON CONFLICT DO NOTHING`, `FOR UPDATE SKIP LOCKED`, and transaction-scoped `pg_advisory_xact_lock(group_id)`.

- [ ] **Step 4: Implement store methods**

All methods return stable wrapped errors without values from chat IDs or snapshots. Claims transition `received` or expired `running` to `running` atomically. Completion updates only the claimed event and clears its lease.

- [ ] **Step 5: Run store tests GREEN**

Run `go test ./internal/store -run Feishu -count=1`. Expected: PASS when `RELAY_OPS_TEST_DATABASE_URL` is available; otherwise existing helper skips with the same baseline behavior.

- [ ] **Step 6: Commit**

```bash
git add relay-ops-service/internal/store/migrations/001_init.sql relay-ops-service/internal/store/postgres.go relay-ops-service/internal/store/postgres_test.go
git commit -m "feat: persist idempotent Feishu command jobs"
```

### Task 7: 实现飞书 OpenAPI 回复客户端

**Files:**
- Create: `relay-ops-service/internal/feishuapi/client.go`
- Create: `relay-ops-service/internal/feishuapi/client_test.go`

**Interfaces:**
- Produces: `NewClient(baseURL, appIDFile, appSecretFile string) (*Client, error)`
- Produces: `SendText(context.Context, chatID, text string) (messageID string, err error)`

- [ ] **Step 1: Write failing fake OpenAPI tests**

Assert token request is `POST /open-apis/auth/v3/tenant_access_token/internal`, message request is `POST /open-apis/im/v1/messages?receive_id_type=chat_id`, body uses `msg_type=text` and a JSON-encoded `content` string, token is reused before `expire-60s`, 401 forces one token refresh, response limit is 1 MiB, and errors never contain app secret/token/chat ID/body.

- [ ] **Step 2: Confirm RED**

Run `go test ./internal/feishuapi -count=1`. Expected: package missing.

- [ ] **Step 3: Implement the minimal client**

Read and trim both secret files once at construction, validate an HTTPS base URL except in `httptest`, cache token under a mutex, set a 10-second HTTP timeout, cap responses, and expose no generic request method.

- [ ] **Step 4: Run package tests GREEN**

Run `go test ./internal/feishuapi -count=1`. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/feishuapi
git commit -m "feat: send deterministic Feishu bot replies"
```

### Task 8: 实现 HTTP 快速接收与命令 worker

**Files:**
- Create: `relay-ops-service/internal/commands/worker.go`
- Create: `relay-ops-service/internal/commands/worker_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/http/server_test.go`

**Interfaces:**
- Consumes: verifier, command store, routing controller, Feishu sender.
- Produces: `commands.NewHTTPHandler(verifier, repository) http.Handler`
- Produces: `Worker.Run(context.Context) error` and `Worker.RunOnce(context.Context) (bool, error)`.

- [ ] **Step 1: Write failing handler tests**

Assert challenge returns JSON immediately; valid event inserts once and returns 200; duplicate returns 200 without another job; invalid auth is 401 without insert; unavailable DB is 503; ignored private/bot/non-text returns 200 without write; unknown group text creates a rejected reply job once; request duration does not include routing or reply calls.

- [ ] **Step 2: Confirm handler RED**

Run `go test ./internal/http ./internal/commands -run 'Feishu|HTTP' -count=1`. Expected: symbols missing.

- [ ] **Step 3: Implement public handler**

Map verifier errors to 401/400/413, challenge to `{"challenge":"..."}`, accepted/rejected user group messages to a single idempotent store insert, and all ignored protocol-valid events to 200. Use `http.MaxBytesReader` with 256 KiB.

- [ ] **Step 4: Write failing worker tests**

Assert `disabled` never calls routing and sends a disabled result; `dry_run` calls `Switch(..., true)`; `enabled` calls `Switch(..., false)`; status is always read-only; same group calls repository lock; completion is saved before reply; reply failure is retried at most three times without rerunning routing; context cancellation stops polling.

- [ ] **Step 5: Confirm worker RED**

Run `go test ./internal/commands -run Worker -count=1`. Expected: missing worker behavior.

- [ ] **Step 6: Implement minimal worker and fixed rendering**

Use a one-second idle poll, 30-second claim lease, one job at a time, fixed Chinese reply templates, a 12-character audit ID derived from event ID, and stable error codes. Save route completion before any reply attempt; reply failures update only delivery state.

- [ ] **Step 7: Run handler and worker tests GREEN**

Run `go test ./internal/http ./internal/commands -count=1`. Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add relay-ops-service/internal/commands relay-ops-service/internal/http/server.go relay-ops-service/internal/http/server_test.go
git commit -m "feat: receive and execute Feishu command jobs"
```

### Task 9: 组装应用、部署契约与运维手册

**Files:**
- Modify: `relay-ops-service/internal/app/app.go`
- Modify: `relay-ops-service/internal/app/app_test.go`
- Modify: `relay-ops-service/cmd/relay-ops/main.go`
- Modify: `infra/compose.yaml`
- Modify: `infra/Caddyfile`
- Modify: `infra/.env.example`
- Modify: `tests/relay_ops/validate_relay_ops_contract.sh`
- Create: `docs/runbooks/feishu-command-control.md`

**Interfaces:**
- Produces: exact public path `POST /relay-ops/api/feishu/events`.
- Produces: command subsystem omitted only when disabled and no callback secrets are configured; otherwise startup is fail-closed on configuration errors.

- [ ] **Step 1: Write failing app and route tests**

Construct a test app with fake dependencies. Assert exact callback path bypasses existing admin session middleware, adjacent paths and non-POST requests do not, disabled default cannot call Sub2API writes, and cancellation stops the worker.

- [ ] **Step 2: Confirm app RED**

Run `go test ./internal/app ./internal/http -run Feishu -count=1`. Expected: missing assembly.

- [ ] **Step 3: Assemble runtime dependencies**

Read secret values only inside verifier/OpenAPI constructors, load routing config, attach the exact handler on the root mux before `/`, expose a `CommandWorker` runner on `App`, and start it alongside the existing scheduler from `main` only when configured.

- [ ] **Step 4: Extend deployment contract before editing deployment files**

Add failing shell assertions for `RELAY_OPS_FEISHU_COMMAND_MODE: ${RELAY_OPS_FEISHU_COMMAND_MODE:-disabled}`, five read-only mounts, exact POST callback matcher, separation from `@relay_ops_admin`, and no host port other than Caddy.

- [ ] **Step 5: Confirm shell RED**

Run `bash tests/relay_ops/validate_relay_ops_contract.sh`. Expected: FAIL on the first missing Feishu command contract.

- [ ] **Step 6: Implement Compose and Caddy configuration**

Mount `/run/secrets/feishu-app-id`, `/run/secrets/feishu-app-secret`, `/run/secrets/feishu-verification-token`, `/run/secrets/feishu-encrypt-key`, and `/run/secrets/feishu-routing.json` read-only. Add an exact method/path Caddy matcher before the admin matcher; exclude that exact path from the admin matcher. Keep relay-ops read-only and unprivileged.

- [ ] **Step 7: Write the runbook**

Document exact Feishu scopes (`im:message:send_as_bot` plus the minimal group-message receive scope shown by the developer console), event `im.message.receive_v1`, callback URL, secret file creation with `install -m 0600`, configuration ID discovery using read-only Admin API, challenge verification, disabled/dry-run/enabled checks, audit SQL, and partial-state manual recovery. Do not include real credentials or IDs.

- [ ] **Step 8: Run app and deployment tests GREEN**

Run app tests in the pinned container, then `bash tests/relay_ops/validate_relay_ops_contract.sh`. Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add relay-ops-service/internal/app/app.go relay-ops-service/internal/app/app_test.go relay-ops-service/cmd/relay-ops/main.go infra/compose.yaml infra/Caddyfile infra/.env.example tests/relay_ops/validate_relay_ops_contract.sh docs/runbooks/feishu-command-control.md
git commit -m "feat: wire Feishu command control deployment"
```

### Task 10: 全量验证、安全审计与 dry-run 交付

**Files:**
- Modify only files proven necessary by failing verification.

**Interfaces:**
- Verifies every completion criterion from the approved design.

- [ ] **Step 1: Format and run focused tests**

Run:
```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 sh -c 'gofmt -w internal/feishuevents internal/feishuapi internal/commands internal/routingcontrol internal/sub2api internal/config internal/store internal/http internal/app cmd/relay-ops && go test ./internal/feishuevents ./internal/feishuapi ./internal/commands ./internal/routingcontrol ./internal/sub2api ./internal/config ./internal/store ./internal/http ./internal/app -count=1'
```
Expected: PASS.

- [ ] **Step 2: Run race, vet, full regression and image build**

Run:
```bash
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 go test ./... -race -count=1
docker run --rm -v "$PWD/relay-ops-service:/src" -w /src golang@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 go vet ./...
docker compose --env-file infra/.env.example -f infra/compose.yaml config --quiet
docker build -f infra/Dockerfile.relay-ops -t relay-ops:feishu-command-test .
bash tests/relay_ops/validate_relay_ops_contract.sh
```
Expected: all commands exit 0.

- [ ] **Step 3: Perform deterministic security scans**

Run:
```bash
rg -n 'exec\.Command|/bin/sh|bash -c|Authorization|app_secret|tenant_access_token|chat_id|open_id' relay-ops-service/internal/{commands,feishuevents,feishuapi,routingcontrol,sub2api}
rg -n 'confirm_mixed_channel_risk|/api/v1/admin/' relay-ops-service/internal/{commands,routingcontrol,sub2api}
```
Expected: no shell execution; secrets appear only as JSON/header field names and never in formatted errors/logs; Sub2API paths are only the five approved read/write endpoints; mixed-channel override is absent.

- [ ] **Step 4: Review the approved spec line by line**

Confirm every goal, non-goal, size limit, sender/chat rule, idempotency rule, switch ordering, partial-state rule, reply retry limit, default mode, secret boundary and rollout gate has a named test. Add a failing regression test before fixing any discovered gap.

- [ ] **Step 5: Route any correction back through its owning task**

If Steps 1-4 expose a defect, return to the task that owns that file, add a
focused failing regression test, implement the smallest correction, rerun that
task's GREEN command and full regression, then use that task's explicit `git
add` list and commit command. If verification is clean, make no empty commit.

- [ ] **Step 6: Configure Feishu and perform disabled/dry-run acceptance**

Use the developer console to add the minimal receive-message permission, subscribe `im.message.receive_v1`, set Encrypt Key and verification token, configure the HTTPS callback, publish the version, and test in a group. Keep `RELAY_OPS_FEISHU_COMMAND_MODE=disabled` until challenge and rejection behavior pass; then switch only to `dry_run` and verify all five commands produce correct predicted state with zero Sub2API writes.

- [ ] **Step 7: Stop before production writes**

Report the exact dry-run evidence, discovered group/account IDs, current route state, test results and remaining production action. Do not set `RELAY_OPS_FEISHU_COMMAND_MODE=enabled` or perform a real switch until the user gives action-time approval after reviewing dry-run evidence.

---

## Plan Self-Review

- **Spec coverage:** Tasks 1-10 cover callback verification, exact parsing, all-group authorization, private/bot exclusion, configuration safety, Sub2API native switching, model preflight, idempotency, leases, group locking, audit, replies, retries, HTTP exposure, deployment, rollout and rollback. No approved behavior is deferred.
- **Placeholder scan:** The plan contains no `TBD`, no implementation-later instruction, no generic error-handling step, no path placeholder, and no undefined cross-task interface.
- **Type consistency:** `sub2api.Controller` is produced in Task 1 and consumed in Task 5; `routingcontrol.Role/Result` is produced in Task 5 and consumed in Tasks 4/8; `commands.Record/Completion` is produced in Tasks 4/8 and consumed by Task 6; the app wiring in Task 9 consumes only previously defined interfaces.
- **Execution choice:** The user already selected immediate inline execution and waived a plan-review pause, so implementation proceeds with `superpowers:executing-plans` without another handoff question.
