# Sub2API 外置定制与官方优先升级实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将星桥定制能力从 Sub2API 官方核心逐步迁移到外置控制面，仅保留可审计的薄适配器，使官方版本可在非活动槽位完成资格验证后快速蓝绿升级，同时保持管理员登录、页面、数据查看和数据口径不变。

**Architecture:** Sub2API 官方核心继续拥有身份、账号、路由、实时请求链、原始计费和核心业务表的唯一写入权。`relay-ops-service` 演进为星桥控制面，使用事务 outbox、幂等消费者和可重建读模型承载监控、盈利、账务、采集、对账和通知；Caddy 保持同域名入口与会话透传。过渡期只在官方核心保留请求完成、健康变化和最小管理命令适配器。宿主发布控制器复用官方 Release 下载、校验和替换逻辑，在非活动槽位执行合同测试、迁移门禁和蓝绿提升。

**Tech Stack:** Go 1.x、Vue 3/TypeScript、Ent、PostgreSQL、Redis、Caddy、Docker Compose、systemd、现有 `sub2api-updater` 与 `ops/*` 本地/宿主脚本链。

## Global Constraints

- 官方 Release 是版本、asset、checksum 和 commit 的事实源。
- 管理员登录、2FA、主域名、菜单、URL、字段、筛选、排序和导出原则上不变。
- 外置服务不得直接写 `accounts`、`groups`、`usage_logs`、余额或计费相关核心表。
- 外置统计、盈利、监控和对账数据必须可重建，并返回 `generated_at`、`source_watermark`、`freshness_seconds`、`completeness`、`calculation_version`。
- 实时路由和计费不得依赖外部网络服务；事件投递只能异步化。
- 新版本必须在非活动槽位通过合同测试和数据对比后才能提升。
- Expand-only 迁移可受控应用；潜在破坏性迁移必须在生产克隆验证；破坏性迁移不得进入自动蓝绿通道。
- 发布、暂存、提升和回退继续使用本地/宿主脚本链，不新增或依赖 GitHub Actions。
- 任何工程事项只有“已推送到服务端 + 已生产部署 + 已验证生效”才能在 `docs/project/project-progress.md` 标记“已完成”。
- 未完成当前适配器收敛前，生产继续使用合格定制镜像，不直接切换官方原版。

---

## 文件与边界总览

### 新建文件

- `docs/contracts/sub2api-customization-inventory.yaml`：定制能力、源码、API、表、页面、事实源和验收证据清单。
- `docs/contracts/sub2api-integration-contract-v1.yaml`：事件、命令、字段、错误码、精度和版本合同。
- `docs/contracts/admin-experience-contract.md`：登录、菜单、URL、筛选、排序、分页、刷新、导出和降级行为合同。
- `upstream/sub2api/backend/internal/events/outbox.go`、`consumer.go`、`types.go`：核心 outbox 写入、事件序列化和合同版本。
- `upstream/sub2api/backend/migrations/200_externalization_outbox.sql`：仅新增 outbox/消费所需结构。
- `relay-ops-service/internal/events/consumer.go`、`contract.go`、`watermark.go`：事件消费、幂等和水位。
- `relay-ops-service/internal/projection/accounts.go`、`profitability.go`、`accounting.go`、`reconciliation.go`：可重建读模型投影。
- `relay-ops-service/internal/store/migrations/013_externalization_read_models.sql`：控制面表、索引、死信和重建任务。
- `relay-ops-service/internal/controlplane/server.go`、`types.go`、`auth.go`、`read_models.go`、`writes.go`：同域管理 API、权限校验和受控写操作。
- `relay-ops-service/internal/adapter/sub2api.go`：官方 API/薄适配命令适配器。
- `upstream/sub2api/backend/internal/integration/events.go`、`commands.go`：核心侧最小事件和命令接口。
- `upstream/sub2api/backend/internal/integration/events_test.go`、`relay-ops-service/internal/events/consumer_test.go`、`relay-ops-service/internal/projection/accounts_test.go`、`relay-ops-service/internal/controlplane/server_test.go`、`relay-ops-service/internal/adapter/sub2api_test.go`：合同、幂等、权限和故障隔离测试。
- `upstream/sub2api/frontend/src/api/controlPlane.ts`、`src/composables/useReadModelFreshness.ts`、`src/components/admin/ReadModelStatus.vue`：控制面 API、数据新鲜度展示和降级组件。
- `sub2api-updater/internal/updater/release_state.go`、`qualification.go`、`migration_gate.go`、对应测试：官方候选状态机、资格和迁移门禁。
- `ops/qualify-sub2api-official-release.sh`、`ops/promote-sub2api-qualified-release.sh`、`ops/rollback-sub2api-release.sh`：本地/宿主资格、提升和回退入口。
- `infra/compose.sub2api-rehearsal.yaml`、`infra/systemd/sub2api-release-qualification.service`、`infra/systemd/sub2api-release-qualification.timer`：非活动槽位和后台资格调度。
- `docs/runbooks/sub2api-externalization-migration.md`、`docs/runbooks/sub2api-official-upgrade.md`：迁移、门禁、验收和回退手册。

### 现有文件修改边界

- `relay-ops-service/internal/sub2api/client.go`、`sync.go`、`types.go`：只扩展官方 API 客户端和服务身份，不引入核心表 SQL。
- `relay-ops-service/internal/adminauth/middleware.go`、`middleware_test.go`：同域管理员会话校验与控制面 401/403 隔离。
- `relay-ops-service/internal/http/server.go`：挂载 `/api/v1/xingqiao/*`，不改变原生 `/api/v1/*`。
- `upstream/sub2api/frontend/src/router/index.ts`、现有账号监控/盈利/账务/对账视图和对应 API 类型：保留原 URL、菜单和字段，增加来源/新鲜度信息并按 feature flag 切读模型。
- `infra/Caddyfile`、`infra/sub2api-update-ui/update-ui.js`：同域反向代理和状态展示；不在前端触发现场编译。
- `sub2api-updater/internal/updater/service.go`、`preparer.go`、`executor.go`、`http.go`、`store.go`：复用官方发现/下载/校验逻辑，增加非活动资格与最终提升门禁。
- `ops/update-sub2api-host.sh`、`ops/deploy-sub2api-blue-green-host.sh`、`ops/merge-sub2api-release.sh`：串接 `discover -> qualify -> stage -> promote`，保留现有无 GitHub Actions 约束。
- `docs/project/project-progress.md`：每个实施单元先登记“进行中”，只有生产证据齐全才改为“已完成”。

---

## Task 1: 冻结当前生产基线并完成定制资产盘点

**Files:**
- Create: `docs/contracts/sub2api-customization-inventory.yaml`
- Create: `docs/contracts/admin-experience-contract.md`
- Create: `tests/contracts/baseline_manifest_test.rb`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Produces `customization-inventory` 清单、当前生产版本/镜像/迁移哈希和管理员体验黄金样本，供所有后续任务使用。

- [ ] **Step 1: Write the failing inventory test**

在 `tests/contracts/baseline_manifest_test.rb` 中校验清单每条记录都包含 `capability`、`owner`、`source_paths`、`api_paths`、`tables`、`ui_routes`、`source_of_truth`、`acceptance_evidence`，且 `owner` 只能是 `core`、`adapter`、`control_plane`、`host`。

- [ ] **Step 2: Run test to verify it fails**

Run: `ruby -Itests tests/contracts/baseline_manifest_test.rb`

Expected: FAIL because the inventory file does not yet exist.

- [ ] **Step 3: Record the inventory and admin contract**

按设计文档第 5、7、10 节记录主页/文档、监控、盈利、账务、对账、余额/倍率采集、请求完成事件和升级发布；每条记录同时列出当前源码路径、API、Ent 表、前端路由、旧实现和目标实现。管理员合同必须明确同域登录、2FA、原 URL、字段、筛选、排序、分页、刷新、CSV 和控制面不可用时的降级。

- [ ] **Step 4: Add the current runtime manifest**

记录当前生产 Sub2API/Caddy/PostgreSQL/Redis/Worker 镜像身份、迁移哈希、活动槽位和 `release-state`，引用现有生产验证报告，不执行生产写操作。

- [ ] **Step 5: Run test to verify it passes**

Run: `ruby -Itests tests/contracts/baseline_manifest_test.rb`

Expected: PASS and report the number of capabilities and required admin routes.

- [ ] **Step 6: Commit**

```bash
git add docs/contracts/sub2api-customization-inventory.yaml docs/contracts/admin-experience-contract.md tests/contracts/baseline_manifest_test.rb docs/project/project-progress.md
git commit -m "docs: freeze sub2api customization and admin baseline"
```

## Task 2: 建立版本化集成合同与核心 outbox

**Files:**
- Create: `docs/contracts/sub2api-integration-contract-v1.yaml`
- Create: `upstream/sub2api/backend/internal/integration/events.go`
- Create: `upstream/sub2api/backend/internal/integration/commands.go`
- Create: `upstream/sub2api/backend/internal/events/types.go`
- Create: `upstream/sub2api/backend/internal/events/outbox.go`
- Create: `upstream/sub2api/backend/migrations/200_externalization_outbox.sql`
- Test: `upstream/sub2api/backend/internal/integration/events_test.go`
- Test: `upstream/sub2api/backend/internal/events/outbox_test.go`

**Interfaces:**
- Produces `integration_contract_version: 1`.
- `Event{EventID, Type, OccurredAt, SourceVersion, ContractVersion, Payload}`。
- `Outbox.Append(ctx, tx, event) error` 与 `Outbox.ClaimBatch(ctx, consumer, limit) ([]Event,error)`。
- `Command{CommandID, ActorID, Name, Payload, ContractVersion}`，核心只接受白名单命令。

- [ ] **Step 1: Write failing contract tests**

覆盖请求完成、账号健康变化、余额快照、账号更新命令；断言事件 ID、合同版本、时间精度、金额使用字符串 decimal、payload 不含 token/cookie/key 字段。

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `cd upstream/sub2api/backend && go test ./internal/integration ./internal/events -run 'Test(Contract|Outbox)' -v`

Expected: FAIL because packages and migration are absent.

- [ ] **Step 3: Implement the contract and transactional outbox**

新增仅扩展结构的迁移：事件表唯一约束 `event_id`、状态/租约索引、合同版本和 JSONB payload；Append 必须与核心业务事务同提交，Claim 必须使用 `FOR UPDATE SKIP LOCKED`，失败进入重试并记录稳定错误类别。禁止在控制面连接核心数据库写业务表。

- [ ] **Step 4: Run focused tests and migration lint**

Run: `cd upstream/sub2api/backend && go test ./internal/integration ./internal/events -v && go vet ./internal/integration ./internal/events`

Expected: PASS; migration review confirms expand-only and no destructive statement.

- [ ] **Step 5: Commit**

```bash
git add docs/contracts/sub2api-integration-contract-v1.yaml upstream/sub2api/backend/internal/integration upstream/sub2api/backend/internal/events upstream/sub2api/backend/migrations/200_externalization_outbox.sql
git commit -m "feat: add versioned integration contract and transactional outbox"
```

## Task 3: 实现控制面事件消费、幂等和可重建读模型

**Files:**
- Create: `relay-ops-service/internal/events/contract.go`
- Create: `relay-ops-service/internal/events/consumer.go`
- Create: `relay-ops-service/internal/events/watermark.go`
- Create: `relay-ops-service/internal/projection/accounts.go`
- Create: `relay-ops-service/internal/projection/profitability.go`
- Create: `relay-ops-service/internal/projection/accounting.go`
- Create: `relay-ops-service/internal/projection/reconciliation.go`
- Create: `relay-ops-service/internal/store/migrations/013_externalization_read_models.sql`
- Test: `relay-ops-service/internal/events/consumer_test.go`
- Test: `relay-ops-service/internal/projection/accounts_test.go`
- Test: `relay-ops-service/internal/projection/profitability_test.go`
- Modify: `relay-ops-service/internal/store/postgres.go`

**Interfaces:**
- `Consumer.Handle(ctx, event) error`：按 `event_id` 幂等。
- `Watermark{Source,LastEventID,OccurredAt,ProcessedAt,Completeness}`。
- `Projection.Rebuild(ctx, snapshot, events) error`。
- 所有读模型行包含 `generated_at`、`source_watermark`、`freshness_seconds`、`completeness`、`calculation_version`。

- [ ] **Step 1: Write failing tests**

测试重复事件只产生一份投影、乱序事件按 occurred_at 和 event_id 稳定处理、断点续消费、死信转移、从快照重建后金额/数量/排名与黄金样本一致。

- [ ] **Step 2: Run tests to confirm failure**

Run: `cd relay-ops-service && go test ./internal/events ./internal/projection ./internal/store -run 'Test(Consumer|Projection|Rebuild)' -v`

Expected: FAIL because the new packages and tables do not exist.

- [ ] **Step 3: Implement schema and consumers**

创建控制面自己的事件游标、投影、死信和重建任务表；金额统一使用 decimal 库/字符串落库，评分和利润公式绑定计算版本；消费者不得执行核心表 SQL。

- [ ] **Step 4: Run full control-plane tests**

Run: `cd relay-ops-service && go test ./... && go vet ./...`

Expected: PASS with explicit completeness and lag assertions.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/events relay-ops-service/internal/projection relay-ops-service/internal/store/migrations/013_externalization_read_models.sql relay-ops-service/internal/store/postgres.go
git commit -m "feat: add idempotent control-plane projections"
```

## Task 4: 建立同域管理员 API、权限隔离和新鲜度响应

**Files:**
- Create: `relay-ops-service/internal/controlplane/server.go`
- Create: `relay-ops-service/internal/controlplane/types.go`
- Create: `relay-ops-service/internal/controlplane/auth.go`
- Create: `relay-ops-service/internal/controlplane/read_models.go`
- Create: `relay-ops-service/internal/controlplane/writes.go`
- Create: `relay-ops-service/internal/adapter/sub2api.go`
- Modify: `relay-ops-service/internal/adminauth/middleware.go`
- Modify: `relay-ops-service/internal/adminauth/middleware_test.go`
- Modify: `relay-ops-service/internal/http/server.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/sync.go`
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Test: `relay-ops-service/internal/controlplane/server_test.go`
- Test: `relay-ops-service/internal/controlplane/auth_test.go`

**Interfaces:**
- `GET /api/v1/xingqiao/accounts/monitor`
- `GET /api/v1/xingqiao/operations/profitability`
- `GET /api/v1/xingqiao/accounting/ledger`
- `GET /api/v1/xingqiao/reconciliation`
- `POST /api/v1/xingqiao/accounts/{id}/refresh`，`Idempotency-Key` 必填。
- `AuthClient.Me(ctx, bearer, clientIP, origin) (AdminIdentity,error)`。

- [ ] **Step 1: Write failing API/auth tests**

断言同域 Bearer 会话调用核心 `/api/v1/auth/me`；非管理员、非活动会话、IP/Origin 不匹配返回 401/403；控制面 401 不触发主站登出；响应包含全部新鲜度字段且不泄露内部服务地址或凭据。

- [ ] **Step 2: Run focused tests to confirm failure**

Run: `cd relay-ops-service && go test ./internal/controlplane ./internal/adminauth ./internal/http -run 'Test(Auth|ReadModel|Write)' -v`

Expected: FAIL before routes and middleware are implemented.

- [ ] **Step 3: Implement read and write boundaries**

读请求只查控制面投影；写请求优先调用官方 API，缺失时调用版本化薄适配命令；服务身份短时有效，审计记录 actor、命令、幂等键、结果和合同版本；绝不把管理员 Bearer 重放给核心写接口。

- [ ] **Step 4: Verify security and API behavior**

Run: `cd relay-ops-service && go test ./... && go test -race ./internal/controlplane ./internal/adminauth`

Expected: PASS; logs contain no Bearer/Cookie/API key literals in test fixtures or request dumps.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/controlplane relay-ops-service/internal/adapter relay-ops-service/internal/adminauth relay-ops-service/internal/http relay-ops-service/internal/sub2api
git commit -m "feat: expose same-origin control-plane APIs with isolated auth"
```

## Task 5: 迁移管理员页面为双读并保持无感体验

**Files:**
- Create: `upstream/sub2api/frontend/src/api/controlPlane.ts`
- Create: `upstream/sub2api/frontend/src/composables/useReadModelFreshness.ts`
- Create: `upstream/sub2api/frontend/src/components/admin/ReadModelStatus.vue`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountProfitability.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/reconciliation.ts`
- Create: `upstream/sub2api/frontend/src/__tests__/controlPlaneApi.spec.ts`

**Interfaces:**
- `useReadModelFreshness(response): { generatedAt, watermark, freshnessSeconds, completeness, calculationVersion, degraded }`。
- 页面通过 feature flag 选择 `legacy_only`、`shadow`、`external_primary`，路由和菜单名称不变。

- [ ] **Step 1: Write failing component/API tests**

使用现有最小、默认、最大时间范围 fixture，断言字段、筛选、排序、分页、刷新、详情和 CSV 列与旧页面一致；控制面不可用时显示局部降级提示，不跳转登录、不清理主站会话。

- [ ] **Step 2: Run frontend tests to confirm failure**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/__tests__/controlPlaneApi.spec.ts`

Expected: FAIL because the control-plane client and status component are absent.

- [ ] **Step 3: Implement dual-read UI**

保持 `/admin/accounts/monitor`、`/admin/operations/account-profitability` 等现有路径；仅新增“更新时间/来源/完整性/计算版本”可解释信息；shadow 模式只上报差异，不改变用户看到的 legacy 结果。

- [ ] **Step 4: Run frontend validation**

Run: `cd upstream/sub2api/frontend && pnpm vitest run && pnpm lint && pnpm typecheck && pnpm build`

Expected: PASS; production bundle contains no second login or internal hostname.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/frontend/src/api/controlPlane.ts upstream/sub2api/frontend/src/composables/useReadModelFreshness.ts upstream/sub2api/frontend/src/components/admin/ReadModelStatus.vue upstream/sub2api/frontend/src/router/index.ts upstream/sub2api/frontend/src/views/admin upstream/sub2api/frontend/src/__tests__
git commit -m "feat: dual-read admin views without changing routes"
```

## Task 6: 迁移余额、账单、倍率采集与受控写操作

**Files:**
- Modify: `relay-ops-service/internal/billing/cost.go`
- Modify: `relay-ops-service/internal/billing/source.go`
- Modify: `relay-ops-service/internal/billing/http_adapter.go`
- Modify: `relay-ops-service/internal/billing/sub2api.go`
- Modify: `relay-ops-service/internal/collection/pricing.go`
- Modify: `relay-ops-service/internal/pricing/fetcher.go`
- Modify: `relay-ops-service/internal/pricing/extractor.go`
- Modify: `relay-ops-service/internal/upstreams/service.go`
- Modify: `relay-ops-service/internal/controlplane/writes.go`
- Create: `relay-ops-service/internal/adapter/sub2api_test.go`
- Create: `relay-ops-service/internal/store/migrations/014_externalization_commands.sql`
- Test: `relay-ops-service/internal/billing/adapter_test.go`
- Test: `relay-ops-service/internal/collection/pricing_test.go`
- Test: `relay-ops-service/internal/upstreams/service_test.go`

**Interfaces:**
- `BalanceSnapshot{AccountID,Amount,Currency,ObservedAt,FreshUntil,Source}`。
- `AccountUpdateCommand{CommandID,ActorID,AccountID,Fields,IdempotencyKey}`。

- [ ] **Step 1: Write failing collector and command tests**

覆盖供应商超时、重复采集、费率变更、余额过期、同一幂等键重放和权限拒绝；断言核心账号只通过官方 API/适配命令更新。

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `cd relay-ops-service && go test ./internal/billing ./internal/collection ./internal/upstreams ./internal/adapter -run 'Test(Balance|Rate|Command)' -v`

Expected: FAIL for the new command and snapshot contracts.

- [ ] **Step 3: Implement collectors and audited commands**

将上游余额/账单/费率写入控制面事实表；按 `ObservedAt/FreshUntil` 判定新鲜度；官方账号倍率、优先级和健康更新通过窄 API；审计和幂等失败不能静默覆盖旧值。

- [ ] **Step 4: Run full tests and static checks**

Run: `cd relay-ops-service && go test ./... && go test -race ./internal/billing ./internal/collection ./internal/controlplane`

Expected: PASS with no direct SQL writes to core business tables.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal relay-ops-service/internal/store/migrations/014_externalization_commands.sql
git commit -m "feat: externalize balance billing and multiplier collection"
```

## Task 7: 将请求完成与健康变化收敛为薄核心适配器

**Files:**
- Modify: `upstream/sub2api/backend/internal/integration/events.go`
- Modify: `upstream/sub2api/backend/internal/integration/commands.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_request.go`
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/service/usage_log_create_result.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/handler/usage_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Create: `upstream/sub2api/backend/internal/integration/request_event_test.go`
- Modify: `relay-ops-service/internal/projection/accounts.go`
- Modify: `relay-ops-service/internal/projection/profitability.go`
- Modify: `relay-ops-service/internal/projection/accounting.go`
- Test: `relay-ops-service/internal/projection/request_event_test.go`

**Interfaces:**
- `RequestCompleted{RequestID,AccountID,RequestedModel,UpstreamModel,ActualResponseModel,InputTokens,OutputTokens,CostUSD,LatencyMS,OccurredAt}`。
- `HealthChanged{AccountID,Status,ErrorCategory,ObservedAt,ProbeVersion}`。

- [ ] **Step 1: Write failing event-contract tests**

断言 JSON/Responses/WebSocket 关键请求链均能生成最小事件；事件包含实际响应模型和成本证据，不包含 prompt、凭据或完整响应体。

- [ ] **Step 2: Run focused backend tests to confirm failure**

Run: `cd upstream/sub2api/backend && go test ./internal/integration ./internal/handler ./internal/service -run 'Test(RequestCompleted|HealthChanged)' -v`

Expected: FAIL until hook points are connected.

- [ ] **Step 3: Add hooks without changing routing decisions**

在事务提交成功后追加 outbox event；事件发送失败只影响异步水位，不阻塞实时请求；不得调用外部控制面进行同步决策。

- [ ] **Step 4: Run protocol and race validation**

Run: `cd upstream/sub2api/backend && go test ./... && go test -race ./internal/integration ./internal/handler ./internal/service`

Expected: PASS; existing routing/charging golden samples remain unchanged.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal relay-ops-service/internal/projection
git commit -m "feat: emit minimal request and health events"
```

## Task 8: 建立官方 Release 背景资格与数据库迁移门禁

**Files:**
- Create: `sub2api-updater/internal/updater/release_state.go`
- Create: `sub2api-updater/internal/updater/qualification.go`
- Create: `sub2api-updater/internal/updater/migration_gate.go`
- Modify: `sub2api-updater/internal/updater/service.go`
- Modify: `sub2api-updater/internal/updater/preparer.go`
- Modify: `sub2api-updater/internal/updater/executor.go`
- Modify: `sub2api-updater/internal/updater/http.go`
- Modify: `sub2api-updater/internal/updater/store.go`
- Test: `sub2api-updater/internal/updater/release_state_test.go`
- Test: `sub2api-updater/internal/updater/qualification_test.go`
- Test: `sub2api-updater/internal/updater/migration_gate_test.go`
- Create: `ops/qualify-sub2api-official-release.sh`
- Create: `ops/promote-sub2api-qualified-release.sh`
- Create: `ops/rollback-sub2api-release.sh`
- Create: `infra/systemd/sub2api-release-qualification.service`
- Create: `infra/systemd/sub2api-release-qualification.timer`
- Modify: `infra/compose.sub2api-rehearsal.yaml`, `infra/Caddyfile`, `infra/sub2api-update-ui/update-ui.js`

**Interfaces:**
- States: `checking`, `qualifying`, `ready`, `blocked`, `promoting`。
- `QualificationReport{Tag,Commit,Asset,Checksum,AdapterVersion,ContractVersion,MigrationClass,Tests,DataDiff,StartedAt,FinishedAt,StableFailure}`。
- `MigrationClass`: `expand_only`、`potentially_breaking`、`destructive`。

- [ ] **Step 1: Write failing state-machine tests**

覆盖官方 Release 元数据锁定、checksum 不匹配、候选准备器缺失、合同测试失败、迁移分类、ready 有效期、promote 超时和健康失败；失败时活动槽位、数据库和 Caddy upstream 必须保持不变。

- [ ] **Step 2: Run updater tests to confirm failure**

Run: `cd sub2api-updater && go test ./... -run 'Test(ReleaseState|Qualification|MigrationGate)' -v`

Expected: FAIL before state and gate implementations exist.

- [ ] **Step 3: Implement background qualification**

复用现有官方发现、下载、平台选择、checksum 和替换逻辑；将合并/编译/迁移分析放到非活动槽位/隔离数据库；破坏性迁移直接 `blocked`；管理员点击只执行最终门禁、切换和健康验证。

- [ ] **Step 4: Add host scripts and systemd scheduling**

脚本以 JSON 记录 tag/commit/asset/checksum、镜像/二进制身份、迁移哈希、合同报告、阶段时间和回退证据；不得创建 `.github/workflows/*`。

- [ ] **Step 5: Run updater, shell, and rehearsal checks**

Run: `cd sub2api-updater && go test ./... && go vet ./...`; `shellcheck ops/qualify-sub2api-official-release.sh ops/promote-sub2api-qualified-release.sh ops/rollback-sub2api-release.sh`; `ops/smoke-sub2api-release.sh --rehearsal`。

Expected: PASS; update UI exposes stage/start/last-progress/stable-failure and disables update when not ready.

- [ ] **Step 6: Commit**

```bash
git add sub2api-updater ops/qualify-sub2api-official-release.sh ops/promote-sub2api-qualified-release.sh ops/rollback-sub2api-release.sh infra/systemd/sub2api-release-qualification.service infra/systemd/sub2api-release-qualification.timer infra/compose.sub2api-rehearsal.yaml infra/Caddyfile infra/sub2api-update-ui/update-ui.js
git commit -m "feat: qualify official releases before blue-green promotion"
```

## Task 9: 完成双读比对、逐页切换和回退演练

**Files:**
- Create: `relay-ops-service/internal/compare/service.go`
- Create: `relay-ops-service/internal/compare/service_test.go`
- Create: `upstream/sub2api/frontend/src/config/externalizationFlags.ts`
- Modify: existing admin views and API clients from Task 5
- Create: `docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- `ReadMode`: `legacy_only`、`shadow_building`、`dual_read_comparing`、`external_primary`、`legacy_retired`。
- `CompareReport{Window,Counts,DecimalAmounts,RateVersions,Freshness,Permission,Export,Degraded,Rollback,Passed}`。

- [ ] **Step 1: Write failing comparison tests**

对最小、默认、最大时间窗逐项比较账号/请求/账单数量、Token、原始成本、收入、采购成本、利润、利润率、余额、倍率、评分、排名和对账异常；金额使用 decimal 精确比较，余额差异必须有采集时间说明。

- [ ] **Step 2: Run comparison tests to confirm failure**

Run: `cd relay-ops-service && go test ./internal/compare -v`

Expected: FAIL until comparator and report persistence exist.

- [ ] **Step 3: Implement feature-flagged cutover**

按“账号监控 → 盈利 → 账务 → 对账”逐页 shadow、双读、外置主路径；每页保留一键切回旧数据源的 flag；达标后再删除旧查询路径。

- [ ] **Step 4: Run UI and rollback rehearsal**

Run: `cd upstream/sub2api/frontend && pnpm vitest run`; `ops/smoke-sub2api-release.sh --rehearsal --rollback`; `cd relay-ops-service && go test ./...`。

Expected: PASS; report records all comparison windows, flags, operator, timestamps and rollback result.

- [ ] **Step 5: Commit**

```bash
git add relay-ops-service/internal/compare upstream/sub2api/frontend/src/config/externalizationFlags.ts docs/superpowers/reports/2026-08-08-externalization-dual-read-report.md docs/project/project-progress.md
git commit -m "feat: gate external read-model cutover with comparisons"
```

## Task 10: 生产分阶段发布、验收与退役深度 fork

**Files:**
- Modify: `docs/runbooks/sub2api-externalization-migration.md`
- Modify: `docs/runbooks/sub2api-official-upgrade.md`
- Modify: `ops/update-sub2api-host.sh`, `ops/deploy-sub2api-blue-green-host.sh`
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-08-externalization-production-verification.md`

**Interfaces:**
- 每个迁移单元必须生成“推送、部署、生效验证”三段证据；未齐全时总账只能标记“进行中”或“工程代码/配置差异待部署”。

- [ ] **Step 1: Stage in non-production/rehearsal**

先部署控制面、outbox 消费者和同域代理到非活动槽位，运行官方 health/readiness、管理员身份、JSON/SSE/Responses/WebSocket、UI 主要交互和数据对比。

- [ ] **Step 2: Run production preflight**

校验 PostgreSQL/Redis/Caddy/Worker 身份、备份点、迁移哈希、活动槽位、`release-state`、证书和回退版本；任何失败不切换流量。

- [ ] **Step 3: Promote one read capability at a time**

按 Task 9 顺序逐页切换，记录操作人、时间、feature flag、健康指标、事件水位、完整性和用户可见异常；控制面异常立即切回，不触碰核心实时请求。

- [ ] **Step 4: Enable official-priority update only after adapter gate**

确认定制能力清单中 `core` 项仅剩薄适配器，且最近三个官方候选均可在背景资格流程达到 `ready`；否则保持合格定制镜像路径。

- [ ] **Step 5: Retire old paths only after evidence**

删除已切换页面的旧查询、旧盈利公式和深度 fork；保留不可变上一版本、读模型重建和旧 schema 兼容证明。禁止删除核心原始数据。

- [ ] **Step 6: Verify and update ledger**

Run: `git diff --check`; all Go/Node/Ruby focused and full tests; `ops/smoke-sub2api-release.sh --production-read-only`; attach production report. 只有服务端推送、部署和线上生效均有证据，才更新 `project-progress.md` 为“已完成”。

- [ ] **Step 7: Commit documentation and evidence**

```bash
git add docs/runbooks docs/superpowers/reports/2026-08-08-externalization-production-verification.md docs/project/project-progress.md ops/update-sub2api-host.sh ops/deploy-sub2api-blue-green-host.sh
git commit -m "docs: record externalization rollout and production gates"
```

---

## 验证矩阵

| 层级 | 必须通过的命令/检查 | 失败处理 |
|---|---|---|
| Go 核心 | `go test ./...`, `go test -race ./...`, `go vet ./...` | 不生成候选，不切活动槽位 |
| Go 控制面 | `go test ./...`, `go test -race ./internal/events ./internal/controlplane` | 保持 legacy 读路径 |
| 前端 | `pnpm vitest run`, `pnpm lint`, `pnpm typecheck`, `pnpm build` | 不切 `external_primary` |
| 数据库 | 隔离克隆迁移、旧版启动、回退兼容证明 | 破坏性迁移标记 `blocked` |
| 发布脚本 | `shellcheck`, rehearsal smoke, promote/rollback dry-run | 活动槽位完全不变 |
| 管理体验 | 登录/2FA、菜单、URL、筛选、排序、分页、刷新、导出、降级 | 切回旧页面/数据源 |
| 生产 | 推送、部署、健康、容器身份、数据对比、线上 UI/API 证据 | 总账维持“进行中” |

## 完成定义与交接

计划完成不等于迁移完成。代码阶段完成只能标为“准备完成”；只有生产推送、生产部署和生效验收三者齐备，才允许在项目总账标记“已完成”。

Plan complete and saved to `docs/superpowers/plans/2026-08-08-sub2api-externalized-customization-implementation-plan.md`. Two execution options:

1. **Subagent-Driven (recommended):** 每个任务分派新鲜子代理，任务后独立审查，最后进行整分支审查。
2. **Inline Execution:** 在当前会话按任务批次执行，并在每个门禁点暂停复核。

实施前必须先确认执行方式；确认后再加载对应执行技能，不在本计划阶段修改运行时代码或生产环境。
