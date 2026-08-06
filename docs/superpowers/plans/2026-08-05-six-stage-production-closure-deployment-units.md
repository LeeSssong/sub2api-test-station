# 六阶段生产收口独立部署单元实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 用户明确要求每个生产部署后暂停并等待验收，本要求覆盖该技能的连续执行默认值。

**Goal:** 将六阶段总控方案拆成可独立审查、推送、部署、回滚和由用户逐个验收的生产单元。

**Architecture:** Task 1 是不改变生产的基线准备；Task 2、3、5、6、7、8、9 是独立生产变更单元。任何包含生产部署或生产配置激活的任务结束时只进入 `awaiting_user_acceptance`，不得自动进入下一任务。Task 4 是只读盘点，用于决定是否需要一个或多个平台适配器子计划。

**Tech Stack:** Git worktree、SDD 持久化台账、Go 1.24、PostgreSQL 18、Docker Compose、Caddy、Vue 3/TypeScript、Vitest、现有 Sub2API 蓝绿发布与 relay-ops 不可变镜像发布脚本。

## Global Constraints

- 所有代理必须先读取 `docs/project/six-stage-production-closure-agent-context.md` 并在报告写 `CONTEXT_ACK=2026-08-05-six-stage-production-closure`。
- 每个任务使用新的实施代理；完成后使用独立任务审查代理；最终使用独立全分支审查代理。
- 只有当前协调任务可以修改 `docs/project/project-progress.md`、合并远端生产基线、执行生产发布或宣布完成。
- 实施代理不得部署、不得修改总账；它只提交任务代码、测试、计划内文档和报告。
- 验证只覆盖当前部署单元及发布脚本要求的安全门禁；禁止运行与本单元无关的全仓库验证。
- Task 2、3、5、6、7、8、9 部署后必须停止，台账写 `awaiting_user_acceptance`；用户确认前禁止生成下一部署任务 brief。
- 账号监控 V3 已完成且冻结职责边界；任何任务不得恢复经营字段或 relay-ops UI。
- 未同时满足“已推送 + 已部署 + 必要线上验证 + 用户验收”的生产单元保持“进行中”。
- 所有秘密只存在于生产 root-owned `0600` 文件，不能进入 Git、报告或工具输出。

---

### Task 1: 固化上下文、收口 Git 基线和校准总账

**Deployment:** No production deployment.

**Files:**
- Coordinator-owned, do not edit in subagent: `docs/project/project-progress.md`
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/active-delivery-contract.md`
- Modify: `docs/project/six-stage-production-closure-agent-context.md`
- Modify: `docs/superpowers/plans/2026-08-05-six-stage-production-closure-master-plan.md`
- Create: `docs/superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md`
- Create: `docs/superpowers/reports/2026-08-05-production-baseline-reconciliation.md`

**Interfaces:**
- Consumes: `origin/main@138d26efa`、`origin/codex/account-monitor-completion@bbfe4a36d`、生产来源提交 `05985e62e...`。
- Produces: 远端 canonical commit、准确总账和 Task 2 可使用的唯一源代码基线。

- [ ] **Step 1: 报告上下文确认**

报告首段写固定 `CONTEXT_ACK`、task brief 绝对路径和 `DEPLOYMENT_GATE=no`。

- [ ] **Step 2: 从账号监控远端分支建立收口分支**

```bash
git fetch --all --prune
git switch -c codex/production-baseline-convergence origin/codex/account-monitor-completion
git merge --no-ff origin/main
```

运行时代码冲突保留账号监控生产分支最新实现；协作规则保留 `origin/main` 最新规则；总账冲突按实际生产证据重写。

- [ ] **Step 3: 做当前任务唯一需要的 Git 验证**

```bash
git merge-base --is-ancestor bbfe4a36d HEAD
git merge-base --is-ancestor 138d26efa HEAD
git merge-base --is-ancestor 05985e62e HEAD
git diff --check
```

不运行 backend、frontend、relay-ops 全量测试；本任务没有运行时代码交付。

- [ ] **Step 4: 修正三个全局状态文件**

在 `current-state.md` 和 `active-delivery-contract.md` 移除已进入账号监控生产基线的过期描述，保留账务运行时、真实账单闭环、营收页、Monitor/飞书和 OpenAI 管理端展示为进行中，并增加“每次部署后等待用户验收”硬门禁。实施代理不得编辑唯一总账；由协调代理依据审查通过的报告更新 `project-progress.md`。

- [ ] **Step 5: 实施代理提交，协调代理在独立审查后推送 canonical 分支**

```bash
git add \
  docs/project/current-state.md \
  docs/project/active-delivery-contract.md \
  docs/project/six-stage-production-closure-agent-context.md \
  docs/superpowers/plans/2026-08-05-six-stage-production-closure-master-plan.md \
  docs/superpowers/plans/2026-08-05-six-stage-production-closure-deployment-units.md \
  docs/superpowers/reports/2026-08-05-production-baseline-reconciliation.md
git commit -m "docs: reconcile production delivery baseline"
```

实施代理不得 push。协调代理在独立审查通过后执行 `git push -u origin codex/production-baseline-convergence`，再受控合并到远端主基线并记录 canonical commit。该步骤不改变生产运行时，因此不触发用户部署验收门，可继续 Task 2。

---

### Task 2: 部署 relay-ops 账务代码，保持 accounting disabled

**Deployment:** Production relay-ops immutable image deployment. Stop after deployment.

**Files:**
- Review/modify only if tests fail: `relay-ops-service/internal/accounting/`
- Review/modify only if tests fail: `relay-ops-service/internal/reconciliation/`
- Review/modify only if tests fail: `relay-ops-service/internal/app/app.go`
- Review/modify only if tests fail: `relay-ops-service/internal/config/`
- Review: `infra/compose.yaml`
- Review: `ops/release-relay-ops.sh`
- Create: `docs/superpowers/reports/2026-08-05-relay-ops-accounting-code-deployment.md`

**Interfaces:**
- Consumes: Task 1 canonical commit。
- Produces: accounting disabled 的新 relay-ops 不可变镜像、已应用向前兼容迁移、在线账务/对账受保护路由。

- [ ] **Step 1: 运行单元范围测试**

```bash
cd relay-ops-service
go test ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store -count=1
go vet ./internal/accounting ./internal/reconciliation ./internal/app ./internal/config ./internal/http ./internal/store
cd ..
bash tests/operations/release_relay_ops_test.sh
bash tests/operations/deploy_relay_ops_host_test.sh
```

不运行无关 relay-ops 包、Sub2API backend 或 frontend 测试。

- [ ] **Step 2: 实施代理提交，独立代理审查**

代码无需修改时，实施报告必须证明 canonical tree 已满足部署要求；不得制造空改动。存在缺陷时只修改上述范围并提交。

- [ ] **Step 3: 协调代理推送并发布不可变镜像**

使用与 canonical tree 绑定的 `0600` 测试证据运行：

```bash
ops/release-relay-ops.sh --mode production --evidence "$RELAY_OPS_EVIDENCE_FILE"
```

生产 secret env 保持 `RELAY_OPS_ACCOUNTING_ENABLED=false`。

- [ ] **Step 4: 必要线上验证**

只验证 relay-ops 镜像身份、`/healthz`、`/readyz`、受保护账务/对账路由认证，以及 PostgreSQL/Redis/Caddy/Sub2API 容器身份未变化。

- [ ] **Step 5: 硬停止等待用户验收**

SDD 台账写：

```text
Task 2: awaiting_user_acceptance (relay-ops accounting code deployed disabled; commit/image/report recorded)
```

不得生成 Task 3 brief。

---

### Task 3: 激活 accounting、执行账本基线和验证日调度

**Deployment:** Production configuration activation and controlled accounting baseline reset. Stop after activation.

**Files:**
- Review: `ops/reset-accounting-baseline.sh`
- Modify: `docs/runbooks/whole-site-accounting-ledger.md`
- Create: `docs/superpowers/reports/2026-08-05-whole-site-accounting-activation.md`

**Interfaces:**
- Consumes: 用户已验收的 Task 2 镜像。
- Produces: `RELAY_OPS_ACCOUNTING_ENABLED=true`、正式 ledger start date、内部流量排除集合和在线 00:10 调度。

- [ ] **Step 1: 运行 reset 脚本专项测试**

```bash
bash tests/operations/reset_accounting_baseline_test.sh
```

- [ ] **Step 2: 只读确定目标和起始日**

记录精确数据库目标、逐表计数、首个允许统计的上海自然日、内部用户 ID 和内部 API Key ID。示例日期 `2026-08-02` 不能直接使用。

- [ ] **Step 3: dry-run、备份和 apply**

```bash
bash ops/reset-accounting-baseline.sh --dry-run --start-date "$LEDGER_START_DATE"
bash ops/reset-accounting-baseline.sh \
  --apply \
  --start-date "$LEDGER_START_DATE" \
  --confirm-ledger-start-date "$LEDGER_START_DATE"
```

应用前暂停写入器；应用后核对账号、分组、用户、API Key、路由和定价表仍存在。

- [ ] **Step 4: 激活并只重建 relay-ops**

设置 accounting 四个环境变量，仅重建 relay-ops，不重建其他服务。

- [ ] **Step 5: 必要线上验证并停止**

验证账务路由、调度注册和“无账单源时阻断日结”的真实状态。台账写 `Task 3: awaiting_user_acceptance`，不得生成 Task 4 brief。

---

### Task 4: 只读盘点全站账单能力并决定平台适配器单元

**Deployment:** No production deployment.

**Files:**
- Create: `docs/superpowers/reports/2026-08-05-billable-account-inventory.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: 用户已验收的 Task 3 accounting runtime。
- Produces: 全站可计费账号分类，以及是否需要 Task 5 平台适配器部署的明确结论。

- [ ] **Step 1: 生成脱敏清单**

只读列出账号 ID、名称、平台、类型、启用/调度状态、分组、真实用量、现有 adapter、`billing_read` 映射和授权状态。

- [ ] **Step 2: 固定四类结论**

每个账号只标记为 `ready`、`adapter_required`、`authorization_required` 或 `not_billable`；不得使用估算成本填补。

- [ ] **Step 3: 独立审查清单**

审查范围只检查是否覆盖所有实际承载付费流量的账号、分类证据是否充分、报告是否脱敏。

- [ ] **Step 4: 生成后续单元**

若没有 `adapter_required`，台账记录 `Task 5: not_required` 并在用户已确认 Task 3 后进入 Task 6。若存在，一个平台生成一份独立 provider-specific 实施计划；每个平台适配器各自实施、审查、部署并等待用户验收，禁止合并成一次大部署。

---

### Task 5: 生成并执行平台专属适配器子计划

**Deployment:** This coordination task does not deploy. Each generated provider-specific child plan deploys exactly one provider and stops for user acceptance.

**Files:**
- Create: `docs/superpowers/plans/2026-08-05-billing-adapter-deployment-index.md`
- Create: one exact provider-specific plan per `adapter_required` platform under `docs/superpowers/plans/`

**Interfaces:**
- Consumes: Task 4 报告中的实际 adapter type、协议样本位置和受影响账号 ID。
- Produces: 文件路径、Go 包名、测试样本和报告名均已写死的平台专属计划；专属计划完成并经用户验收后才返回本计划。

- [ ] **Step 1: 建立适配器部署索引**

索引逐个平台记录实际 adapter type、受影响账号 ID、合法只读协议样本位置、专属计划路径、部署提交、镜像和用户验收状态。索引不得记录凭据或原始账单内容。

- [ ] **Step 2: 为每个平台写无占位符专属计划**

每份计划必须写明实际 Go 文件名、实际 adapter 注册键、实际 fixture 文件名、实际报告名和以下专项门禁：正常交易、累计快照、分页、重复交易、缺字段、401、登录页、限流、币种不符和请求 ID 缺失。

- [ ] **Step 3: 每个平台独立使用 SDD 执行**

专属计划只运行 `go test ./internal/billing -count=1`、`go vet ./internal/billing` 和 relay-ops 发布安全门禁；部署后写 `awaiting_user_acceptance` 并停止。用户确认后更新索引，再执行下一个平台。

- [ ] **Step 4: 所有必要平台通过后返回主计划**

如果 Task 4 没有 `adapter_required`，索引明确写 `no provider adapter deployment required`。只有索引中所有必要平台均为 `user accepted`，Task 6 才可派发。

---

### Task 6: 配置真实账单授权和映射，形成首个非零闭环

**Deployment:** Production root-only billing-source provisioning and first real close. Stop after activation.

**Files:**
- Review: `ops/provision-billing-source-host.sh`
- Review: `relay-ops-service/cmd/provision-billing-source/`
- Review: `relay-ops-service/internal/reconciliation/`
- Modify: `docs/runbooks/whole-site-accounting-ledger.md`
- Create: `docs/superpowers/reports/2026-08-05-billing-source-production-closure.md`

**Interfaces:**
- Consumes: 用户已验收的所有必要 adapter 和合法只读授权。
- Produces: 唯一 `billing_read` 映射、真实上游数据和第一份闭合非零每日快照。

- [ ] **Step 1: 只运行 provisioning/reconciliation 专项测试**

```bash
cd relay-ops-service
go test ./cmd/provision-billing-source ./internal/billing ./internal/reconciliation ./internal/store -count=1
cd ..
bash tests/operations/provision_billing_source_host_test.sh
```

- [ ] **Step 2: 每个 ready 账号独立 root-only provisioning**

声明文件 root-owned `0600`，只引用 bearer 文件名；输出只记录 configured/already_configured、upstream ID 和 billing account ID。

- [ ] **Step 3: 首次采集、对账和 daily close**

核对账单授权会话、上游成本快照、对账请求明细、对账执行记录、每日账务快照五类数据；至少一个核心金额真实非零并可追溯。

- [ ] **Step 4: 必要线上验证并停止**

验证幂等重跑、pending/conflict 阻断确定利润和授权失效状态。台账写 `Task 6: awaiting_user_acceptance`。

---

### Task 7: 独立营收页面部署

**Deployment:** Sub2API frontend/API integration deployment. Stop after deployment.

**Files:**
- Create: `upstream/sub2api/frontend/src/views/admin/RevenueView.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/__tests__/RevenueView.spec.ts`
- Create: `upstream/sub2api/frontend/src/components/admin/revenue/`
- Create: `upstream/sub2api/frontend/src/api/admin/accounting.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/reconciliation.ts`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`

**Interfaces:**
- Consumes: 用户已验收的 Task 6 真实账务数据。
- Produces: `/admin/revenue`，并保持 `/admin/accounts/monitor` 无经营字段。

- [ ] **Step 1: 原型与边界测试**

先交付 V1 HTML 原型供用户确认；测试锁定账号监控无经营字段、营收页管理员权限和 relay-ops 401 不清主站会话。

- [ ] **Step 2: 实现全站/分组/账号经营视图**

展示用户实际计费、上游真实扣费、账号利润、利润率、成本覆盖率、待对账数量、历史和异常；未闭合时显示待对账，不显示伪利润。

- [ ] **Step 3: 只运行营收页专项验证**

```bash
cd upstream/sub2api/frontend
npm run test:unit -- --run src/views/admin/__tests__/RevenueView.spec.ts src/api/__tests__/admin.reconciliation.spec.ts
npm run type-check
npm run build
```

- [ ] **Step 4: 独立审查、蓝绿部署、线上页面验证并停止**

只验证 `/admin/revenue`、三个范围、真实非零字段、会话隔离和账号监控边界。台账写 `Task 7: awaiting_user_acceptance`。

---

### Task 8: Monitor 当前状态与飞书告警/恢复部署

**Deployment:** relay-ops alert acceptance command/runtime deployment. Stop after deployment.

**Files:**
- Create: `relay-ops-service/cmd/monitor-alert-acceptance/main.go`
- Create: `relay-ops-service/cmd/monitor-alert-acceptance/main_test.go`
- Review/modify: `relay-ops-service/internal/accounthealth/`
- Review/modify: `relay-ops-service/internal/notify/`
- Create: `docs/superpowers/reports/2026-08-05-monitor-feishu-production-closure.md`

**Interfaces:**
- Consumes: canonical tree 中的 `a1a10cdff` 最新状态修复。
- Produces: 同一最新探测语义下的 Monitor、告警、恢复和日报，以及 root-only 验收命令。

- [ ] **Step 1: 只运行状态与通知专项测试**

```bash
cd relay-ops-service
go test ./cmd/monitor-alert-acceptance ./internal/accounthealth ./internal/notify ./internal/dailyreport ./internal/app -count=1
```

- [ ] **Step 2: 实现并审查 root-only 验收命令**

命令只投递合成“飞书链路验收”告警/恢复，不读取或修改真实账号、路由或账单；相同 transition 重放必须去重。

- [ ] **Step 3: 部署 relay-ops 并执行真实飞书告警/恢复**

验证最新成功不继承历史 `balance_exhausted`、告警和恢复各一次、第三次恢复去重、链接指向 `/admin/accounts/monitor`。

- [ ] **Step 4: 停止等待用户验收**

台账写 `Task 8: awaiting_user_acceptance`。

---

### Task 9: OpenAI 实际响应模型审计部署

**Deployment:** Sub2API API/worker/frontend deployment. Stop after deployment.

**Files:**
- Review/modify: `upstream/sub2api/backend/migrations/193_usage_log_actual_response_model.sql`
- Review/modify: `upstream/sub2api/backend/internal/service/openai_response_model_audit.go`
- Review/modify: `upstream/sub2api/backend/internal/service/openai_gateway_*`
- Review/modify: `upstream/sub2api/backend/internal/service/openai_ws_v2/`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers.go`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`

**Interfaces:**
- Consumes: canonical tree 已存在的 OpenAI 审计后端基础。
- Produces: requested model 与 actual response model 双字段管理端展示。

- [ ] **Step 1: 逐文件移植缺失面**

不得整体合并本地 `main` 或 `122d293db`；只移植 canonical tree 缺少的 DTO、前端列、类型、文案和测试。

- [ ] **Step 2: 只运行 OpenAI 审计专项验证**

```bash
cd upstream/sub2api/backend
go test ./migrations ./ent/schema ./internal/service ./internal/handler/dto ./internal/handler/admin -count=1
cd ../frontend
npm run test:unit -- --run src/components/admin/usage/__tests__/UsageTable.spec.ts
npm run type-check
npm run build
```

- [ ] **Step 3: 独立审查、蓝绿部署和三协议线上验证**

分别验证 JSON、SSE、Responses WebSocket；客户端响应不变，数据库写入实际模型，管理端显示两列。临时测试 Key 验收后禁用。

- [ ] **Step 4: 停止等待用户最终验收**

台账写 `Task 9: awaiting_user_acceptance`。用户确认后才进行最终全分支审查和总账“已完成”收口。

## Plan Self-Review

- 九个任务把原六点拆成七个独立生产变更单元和两个非部署准备单元。
- 每个生产变更单元都有明确停止点和用户验收门。
- 验证命令均限定在当前单元；仅保留构建、迁移、健康、身份和回滚所需门禁。
- 所有代理通过上下文合同、task brief、SDD ledger 和报告 `CONTEXT_ACK` 恢复状态。
- Task 5 不含平台占位符；它要求根据 Task 4 的实际清单生成文件名和注册键均写死的独立子计划。
