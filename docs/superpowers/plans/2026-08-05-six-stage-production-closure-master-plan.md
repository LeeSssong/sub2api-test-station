# 六阶段生产收口总控实施计划

> **执行切分更新（2026-08-05）：** 用户确认后进一步要求“每个独立部署单元部署完成即暂停，用户验收通过才进入下一个”。实际执行以 [独立部署单元实施计划](2026-08-05-six-stage-production-closure-deployment-units.md) 和 [代理上下文合同](../../project/six-stage-production-closure-agent-context.md) 为准；本文件继续保存六阶段总体架构与完整范围。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 每个任务由新的实施子代理执行，任务完成后由独立审查子代理检查；六个任务全部结束后再做一次全分支审查。

**Goal:** 以当前账号监控生产基线为起点，依次完成代码基线收口、全站账务运行时、真实账单闭环、独立营收页面、Monitor/飞书闭环和 OpenAI 实际响应模型审计，并让全局总账重新与真实生产状态一致。

**Architecture:** 采用“一个远端基线、一个生产任务、一个验收结果”的串行交付方式。`origin/codex/account-monitor-completion` 先作为已部署事实来源与 `origin/main` 收敛；后续每个阶段只从前一阶段已推送、已部署、已验证的远端提交派生。relay-ops 继续承担后台采集、对账、日结和飞书出站，Sub2API 管理端只承载账号监控、营收页面和使用日志展示。

**Tech Stack:** Git worktree、Go 1.24、PostgreSQL 18、Redis、Docker Compose、Caddy、Vue 3/TypeScript、Vitest、Shell/Ruby 发布门禁、现有 Sub2API 蓝绿发布与 relay-ops 不可变镜像发布脚本。

## Global Constraints

- 账号监控 V3 已完成，`/admin/accounts/monitor` 不再增加营收、利润、账务、对账或运营内容。
- 六个阶段严格串行；任一阶段未满足“已推送 + 已部署 + 已验证生效 + 用户验收”，下一阶段不得开始生产实施。
- 实施前由协调任务把 `docs/project/project-progress.md` 对应事项登记为“进行中”；只有生产三项证据和用户验收齐全后才能改为“已完成”。
- 当前已知生产账号监控运行时来源提交为 `05985e62ec88b04d1e647a815eecdb1cf1155776`；生产收口分支当前为 `origin/codex/account-monitor-completion@bbfe4a36d`。
- 2026-08-05 核对时，`origin/main@138d26efa` 与账号监控分支分叉为“main 独有 4 个提交、账号监控分支独有 81 个提交”；本地主工作区 `main` 另有未推送提交，禁止整分支合并到生产基线。
- OpenAI 审计的后端基础提交 `c53bbdf48`、`f20ab6a99`、`7518ac689` 已在账号监控分支历史中；本地提交 `45921782a`、`122d293db` 不得整批盲目 cherry-pick，必须按文件和测试移植。
- 所有账单 Cookie、Bearer、API Key、数据库 URL 和飞书凭据只存在于生产 root-owned `0600` 文件中，不写入 Git、日志、报告或命令输出。
- 不进行付款、购买、充值、实名、商户申请或新的上游消费；真实账单接入只使用用户已经合法持有的只读授权。
- 涉及清账、数据库删除或生产写入时，必须先完成只读预检、备份、精确目标确认和回滚演练。
- 任何生产发布必须记录：远端提交 SHA、不可变镜像/容器身份、线上页面/API/数据或飞书验证结果。

## Delivery Map

| 阶段 | 交付结果 | 前置条件 | 完成门槛 |
|---|---|---|---|
| 1 | 唯一远端生产基线与校准后的总账 | 账号监控生产事实已确认 | 远端基线包含双方历史，文档无过期状态，生产身份未被改写 |
| 2 | 全站账务 relay-ops 运行时已部署并激活 | 阶段 1 完成 | 不可变镜像运行、迁移成功、账务路由与日调度在线 |
| 3 | 全站可计费账号具备合法账单授权、明确映射和真实非零数据 | 阶段 2 完成 | 五类核心账务数据出现真实非零闭环，日结不再因缺源阻断 |
| 4 | 独立 `/admin/revenue` 营收页面 | 阶段 3 连续稳定产生数据 | 全站/分组/账号经营数据可读，账号监控页保持纯净 |
| 5 | Monitor 当前状态与飞书告警/恢复闭环 | 阶段 2 的 relay-ops 新镜像在线 | 最新成功不继承历史失败，真实飞书告警与恢复均投递 |
| 6 | OpenAI 实际响应模型审计上线 | 阶段 5 完成 | JSON/SSE/WebSocket 均写入实际模型，管理端可见且响应不变 |

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
- Consumes: `origin/main@138d26efa`、`origin/codex/account-monitor-completion@bbfe4a36d`、生产运行时来源提交 `05985e62e...`。
- Produces: 供协调代理推送的远端 canonical commit；Task 2 只能从该提交派生。

- [ ] **Step 1: 报告上下文确认**

报告首段必须写 `CONTEXT_ACK=2026-08-05-six-stage-production-closure`、task brief 绝对路径和 `DEPLOYMENT_GATE=no`。

- [ ] **Step 2: 从账号监控远端分支建立收口分支**

从已部署事实分支创建收口分支，不从本地 `main` 创建：

```bash
git fetch --all --prune
git switch -c codex/production-baseline-convergence origin/codex/account-monitor-completion
git merge --no-ff origin/main
```

冲突处理规则：运行时代码优先保留账号监控生产分支的最新实现；`origin/main` 独有的协作规则与已完成登记保留；相互矛盾的总账文字全部按生产证据重写，不能简单选择任一侧。

- [ ] **Step 3: 运行唯一需要的 Git 验证**

```bash
git merge-base --is-ancestor bbfe4a36d HEAD
git merge-base --is-ancestor 138d26efa HEAD
git merge-base --is-ancestor 05985e62e HEAD
git diff --check
```

Expected: 三个祖先检查和 `git diff --check` 均退出 0。不运行 backend、frontend 或 relay-ops 全量测试；本任务没有运行时代码交付。

- [ ] **Step 4: 校准状态入口**

实施代理不编辑唯一总账；协调代理依据审查通过的报告更新它。`current-state.md` 和 `active-delivery-contract.md` 移除已进入账号监控生产基线的过期描述，保留账务运行时、真实账单闭环、营收页、Monitor/飞书和 OpenAI 管理端展示为进行中，并写明每次生产部署后必须等待用户验收。

`current-state.md` 更新到 2026-08-05，并写明：

- 当前生产 Sub2API 来源提交与活动槽位。
- 当前生产 relay-ops 镜像/提交身份。
- canonical Git 提交与生产运行时之间的关系。
- 六阶段剩余顺序及每阶段完成门槛。

- [ ] **Step 5: 写基线收口报告**

报告必须包含以下命令的输出摘要，禁止包含秘密值：

```bash
git rev-list --left-right --count origin/main...origin/codex/account-monitor-completion
git log --oneline origin/codex/account-monitor-completion..origin/main
git log --oneline origin/main..origin/codex/account-monitor-completion
git diff --check
```

- [ ] **Step 6: 实施代理提交，协调代理审查后推送**

实施代理只提交计划内文档和报告，禁止 push 或部署。协调代理在独立审查通过后推送 `codex/production-baseline-convergence`，再受控合并远端主基线并记录 canonical commit；该步骤不改变生产运行时，不触发用户部署验收门。

---

### Task 2: 部署并激活全站账务 relay-ops 运行时

**Files:**
- Review: `relay-ops-service/internal/accounting/`
- Review: `relay-ops-service/internal/reconciliation/`
- Review: `relay-ops-service/internal/store/migrations/008_accounting_ledger.sql`
- Review: `relay-ops-service/internal/store/migrations/009_upstream_cost_reconciliation.sql`
- Review: `relay-ops-service/internal/store/migrations/010_billing_account_mapping.sql`
- Review: `relay-ops-service/internal/store/migrations/011_reconciliation_group_scope.sql`
- Review: `infra/compose.yaml`
- Review: `ops/release-relay-ops.sh`
- Review: `ops/reset-accounting-baseline.sh`
- Modify: `docs/runbooks/whole-site-accounting-ledger.md`
- Create: `docs/superpowers/reports/2026-08-05-whole-site-accounting-production-activation.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: Task 1 的 `CANONICAL_COMMIT`。
- Produces: 运行中的 relay-ops 不可变镜像、可访问的受保护账务/对账 API、启用的 00:10 日调度；Task 3 和 Task 4 依赖这些能力。

- [ ] **Step 1: 登记进行中并执行只读生产预检**

核对发布锁、当前 relay-ops release-state、当前镜像、PostgreSQL/Redis/Caddy/Sub2API 容器身份、迁移表、账务环境变量和五类核心表计数。任何预检不输出数据库 URL 或凭据。

- [ ] **Step 2: 从同一代码树生成测试证据**

```bash
cd relay-ops-service
go test ./... -count=1
go vet ./...
cd ..
bash tests/operations/reset_accounting_baseline_test.sh
bash tests/operations/release_relay_ops_test.sh
bash tests/operations/deploy_relay_ops_host_test.sh
bash tests/relay_ops/validate_relay_ops_contract.sh
```

使用 `ops/write-relay-ops-test-evidence.sh` 生成绑定 `CANONICAL_COMMIT`、tree 和 migrations hash 的 `0600` 证据文件。

- [ ] **Step 3: 先部署账务代码，保持 accounting disabled**

将测试证据固定写入当前 worktree 的 `.release/relay-ops-accounting-activation.json`，权限设为 `0600`，然后发布：

```bash
RELAY_OPS_EVIDENCE_FILE="$(pwd -P)/.release/relay-ops-accounting-activation.json"
chmod 0600 "$RELAY_OPS_EVIDENCE_FILE"
ops/release-relay-ops.sh --mode production --evidence "$RELAY_OPS_EVIDENCE_FILE"
```

首次发布只验证：迁移成功、relay-ops `/healthz` 与 `/readyz` 正常、PostgreSQL/Redis/Caddy/双槽 API/worker 容器身份不变、受保护账务路由存在但未开始日结。

- [ ] **Step 4: 确定账本起始日并完成备份**

起始日必须等于“首个允许进入正式营收统计的上海自然日”，不得沿用运行手册中的示例日期 `2026-08-02`。在报告中记录选择依据、内部用户/API Key 排除清单和备份文件哈希。

先运行：

```bash
bash ops/reset-accounting-baseline.sh --dry-run --start-date "$LEDGER_START_DATE"
```

只有目标数据库、逐表计数、保留表和备份目录全部确认后，才可在暂停写入器的维护窗口执行 `--apply`；应用后立即核对账号、分组、用户、API Key、路由和定价数据未被删除。

- [ ] **Step 5: 启用 accounting 并只重建 relay-ops**

在生产 root-owned secret env 中设置：

```bash
RELAY_OPS_ACCOUNTING_ENABLED=true
RELAY_OPS_ACCOUNTING_LEDGER_START_DATE="$LEDGER_START_DATE"
RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS="$INTERNAL_USER_IDS"
RELAY_OPS_ACCOUNTING_INTERNAL_API_KEY_IDS="$INTERNAL_API_KEY_IDS"
```

其中三个变量必须从本阶段只读预检报告逐字复制；执行脚本在变量为空或与报告哈希不一致时停止。

随后使用同一不可变镜像只重建 relay-ops；不得重建 PostgreSQL、Redis、Caddy、Sub2API 双槽或 worker。

- [ ] **Step 6: 在线验证账务运行时**

验证管理员会话下：

- `GET /relay-ops/accounting` 返回 200。
- `GET /relay-ops/api/accounting/daily?date=$LEDGER_START_DATE` 在尚无快照时返回明确 404，在日结后返回快照。
- `GET /relay-ops/api/reconciliation/operations`、`history`、`exceptions` 均返回结构化 JSON。
- 未登录请求保持 401/403，且不会清除 Sub2API 主站管理员会话。
- 日调度注册成功；在 Task 3 尚无账单源时，日结因“缺少账单源”明确阻断，不能生成伪快照。

- [ ] **Step 7: 回滚门禁**

若迁移、健康、认证或共享容器身份任一失败，`deploy-relay-ops-host.sh` 必须恢复上一不可变镜像。数据库已应用的向前兼容迁移保留，不进行手工降级删除。

- [ ] **Step 8: 审查、推送证据并更新总账**

本阶段只有在远端提交、生产 relay-ops release-state 和线上路由/调度证据齐全时标记“全站账务运行时代码已完成”；“真实账单闭环”仍保持进行中。

---

### Task 3: 接入合法只读账单授权、账号映射与真实非零闭环

**Files:**
- Review: `relay-ops-service/internal/billing/`
- Review: `relay-ops-service/cmd/provision-billing-source/`
- Review: `ops/provision-billing-source.sh`
- Review: `ops/provision-billing-source-host.sh`
- Review: `relay-ops-service/internal/reconciliation/`
- Modify: `docs/runbooks/whole-site-accounting-ledger.md`
- Create: `docs/superpowers/reports/2026-08-05-billing-source-production-closure.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: Task 2 的在线 relay-ops、生产 Sub2API 账号清单、用户合法提供的只读账单授权。
- Produces: 每个可计费账号唯一的 `billing_read` 映射，以及账单授权会话、上游成本快照、对账请求明细、对账执行记录、每日账务快照五类真实数据。

- [ ] **Step 1: 生成全站可计费账号清单**

只读查询生产 `accounts`、分组关系和上游平台信息，输出不含凭据的清单：账号 ID、账号名称、平台、类型、启用状态、所属原生分组、是否产生过真实用量、是否已有 `billing_read` 映射、适配器类型和授权状态。

范围规则：所有启用且产生计费流量、或者配置为可承载计费流量的账号都必须被分类；不能只处理 Neko、Wawazz 或任一供应商。

- [ ] **Step 2: 按能力分类账单源**

每个账号只能进入以下一种状态：

1. `ready`：已有合法只读 Bearer/Cookie 和受支持账单接口。
2. `adapter_required`：有合法只读接口，但现有 adapter 不能解析其交易/快照。
3. `authorization_required`：接口受支持，但尚未取得用户合法授权。
4. `not_billable`：该账号不会承载付费流量，需在调度配置中保持不可用或明确排除。

不得用账号倍率、`today_stats`、采购成本或前端估算替代真实上游扣费。

- [ ] **Step 3: 为缺失协议适配器写失败测试并实现**

仅对 `adapter_required` 平台新增 adapter。测试必须覆盖：正常交易列表、累计快照、分页、重复交易、缺字段、401、登录页、限流、币种不符和请求 ID 缺失。adapter 输出统一为现有 `billing.UsageEvidence` / reconciliation transaction 接口，不修改账号监控评分逻辑。

- [ ] **Step 4: root-only provisioning 每个账单源**

每个声明文件必须是 root-owned `0600`，只引用 bearer 文件名，不包含 bearer 内容。生产执行固定为：

每个执行回合只处理一份已在清单中确认的绝对声明路径，并先导出 `BILLING_DECLARATION`。随后执行：

```bash
RELAY_OPS_IMAGE_DIGEST="$(sudo ruby -rjson -e 'print JSON.parse(File.binread(ARGV.fetch(0))).fetch("current_image")' /var/lib/sub2api/release-records/relay-ops-state.json)"
BILLING_ACCOUNT_ID="$(ruby -rjson -e 'print JSON.parse(File.binread(ARGV.fetch(0))).dig("billing", "billing_account_id")' "$BILLING_DECLARATION")"
sudo /usr/local/libexec/provision-billing-source-host.sh \
  --image "$RELAY_OPS_IMAGE_DIGEST" \
  --declaration "/opt/sub2api/production/secrets/billing-sources/${BILLING_ACCOUNT_ID}.json"
```

每个账号验证输出只允许出现 `configured` 或 `already_configured`、upstream ID 和 billing account ID。迁移 010 的唯一映射约束必须阻止同一账单账号出现两个活动 `billing_read` 会话。

- [ ] **Step 5: 执行首次全站采集与对账**

对每个 `ready` 账号执行一次受控刷新，再执行全站 24 小时 sweep。核对：

- 每个账单源至少产生一条最新成本快照或一组真实交易。
- 同一上游请求/交易不会重复计入。
- 本站请求按唯一 request ID 与上游交易匹配。
- 待匹配、冲突和人工调整分别进入明确状态。
- 任何未闭合记录都会阻断日结，不会生成确定利润。

- [ ] **Step 6: 形成第一份真实非零每日快照**

在一个已完整闭合的上海自然日运行 daily close，并验证：用户实际计费、上游真实扣费、资源成本、现金事件、账号利润/经营毛利相关底层字段为真实数据；至少一个核心金额为非零，且能追溯到本站 usage log 与上游证据。

- [ ] **Step 7: 连续三次调度稳定性验证**

至少观察三次调度周期：重复采集保持幂等，迟到交易能够重算最近三天，授权失效会生成去重事件并停止伪造数据，恢复授权后采集继续。

- [ ] **Step 8: 审查、脱敏报告和总账更新**

报告只记录账号 ID、适配器、映射状态、记录数量、金额汇总、失败分类和证据哈希。所有可计费账号完成分类，所有实际承载付费流量的账号完成授权/映射，五类数据形成真实非零闭环后，本阶段才标记完成。

---

### Task 4: 实施独立营收页面

**Files:**
- Create: `upstream/sub2api/frontend/src/views/admin/RevenueView.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/__tests__/RevenueView.spec.ts`
- Create: `upstream/sub2api/frontend/src/components/admin/revenue/RevenueSummaryCards.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/revenue/RevenueScopeTabs.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/revenue/RevenueHistoryTable.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/revenue/ReconciliationExceptionsPanel.vue`
- Create: `upstream/sub2api/frontend/src/api/admin/accounting.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/reconciliation.ts`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Test: `upstream/sub2api/frontend/src/api/__tests__/admin.reconciliation.spec.ts`
- Create: `docs/prototypes/revenue-v1/index.html`
- Create: `docs/prototypes/revenue-v1/design-qa.md`
- Create: `docs/superpowers/reports/2026-08-05-revenue-page-production-verification.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: `GET /relay-ops/api/accounting/daily`、`GET /relay-ops/api/reconciliation/operations`、`history`、`exceptions`、`POST /refresh`、`POST /exceptions/{id}/adjust`。
- Produces: 独立管理员入口 `/admin/revenue`；账号监控页面不引用任何经营字段。

- [ ] **Step 1: 先写失败的边界测试**

测试必须锁定：

- `/admin/accounts/monitor` 不出现营收、用户计费、利润率、待对账、流水和调整入口。
- `/admin/revenue` 需要管理员权限并出现在侧边栏。
- relay-ops 401 使用 `skipSessionRecovery: true`，不得把账务后台认证失败误判为主站登录失效。

- [ ] **Step 2: 完成 V1 HTML 原型并由用户确认**

原型固定为：顶部范围 Tab（全站/分组/账号）、时间范围、六张核心卡片（用户实际计费、上游真实扣费、账号利润、利润率、成本覆盖率、待对账数量）、历史趋势/累计表、对账异常区。对账未闭合时金额卡显示“待对账”或“对账异常”，不显示伪确定利润。

- [ ] **Step 3: 实现类型安全 API 层**

`accounting.ts` 定义每日快照和现金事件类型；`reconciliation.ts` 复用现有 `OperationsScope`，统一传递 `group_id`、`account_id`、`start`、`end`、`currency=USD`、`timezone=Asia/Shanghai`。所有 relay-ops 请求继续使用网关 URL 和 `skipSessionRecovery: true`。

- [ ] **Step 4: 实现页面范围和数据状态**

全站范围不传 group/account；分组范围只传 `group_id`；账号范围传 `account_id` 并同步所属分组。页面加载必须把 accounting snapshot、operations summary、history 和 exceptions 组合成同一稳定快照；任一关键请求失败时保留上次完整快照并显示“数据已过期”。

- [ ] **Step 5: 实现经营口径展示**

- 用户实际计费：本站真实外部用户扣费。
- 上游真实扣费：已采集并闭合的上游交易/快照。
- 账号利润：用户实际计费减上游真实扣费。
- 利润率：账号利润除以用户实际计费；分母为 0 时显示 `—`。
- 成本覆盖率：已闭合上游成本除以应对账成本；分母为 0 时显示 `—`。
- 待对账数量：pending + conflict，不与已闭合记录混算。

- [ ] **Step 6: 实现异常处理但不恢复 relay-ops UI**

异常面板只调用受保护 API：管理员可刷新对账、查看异常、对明确异常新增幂等人工调整；原始记录不可删除或覆盖。页面不得暴露 Cookie、Bearer、上游凭据或数据库内部错误。

- [ ] **Step 7: 桌面和移动端验收**

在 1440×1000 和 390×844 验证无横向溢出、Tab/卡片/表格层级清晰、待对账状态醒目、账号监控页面未被改动。浏览器控制台无错误或警告。

- [ ] **Step 8: 测试、审查、发布和线上验证**

```bash
cd upstream/sub2api/frontend
npm run test:unit -- --run src/views/admin/__tests__/RevenueView.spec.ts src/api/__tests__/admin.reconciliation.spec.ts
npm run type-check
npm run build
```

发布使用现有 Sub2API 蓝绿流程。线上必须验证 `/admin/revenue` 真实非零数据、全站/分组/账号切换、异常状态和主站会话隔离；随后才能标记完成。

---

### Task 5: 完成 Monitor 当前状态与飞书告警/恢复闭环

**Files:**
- Create: `relay-ops-service/cmd/monitor-alert-acceptance/main.go`
- Create: `relay-ops-service/cmd/monitor-alert-acceptance/main_test.go`
- Review: `relay-ops-service/internal/accounthealth/classify.go`
- Review: `relay-ops-service/internal/accounthealth/classify_test.go`
- Review: `relay-ops-service/internal/notify/`
- Review: `relay-ops-service/internal/store/notification_consolidation.go`
- Review: `relay-ops-service/internal/app/group_availability.go`
- Modify: `docs/superpowers/plans/2026-07-31-monitor-status-feishu-alert-fix-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-08-05-monitor-feishu-production-closure.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: 已在账号监控分支历史中的 `a1a10cdff` 最新状态修复，以及 Task 2 已部署的 relay-ops 新镜像。
- Produces: Monitor 页面当前状态、飞书主动告警、恢复通知和日报使用同一“最新有效探测”语义。

- [ ] **Step 1: 先证明代码和运行镜像一致**

确认 `a1a10cdff` 是 canonical commit 的祖先，Task 2 的 relay-ops 镜像 label 指向包含该提交的 tree；若不是，停止验收并先发布正确镜像。

- [ ] **Step 2: 运行最新状态回归**

```bash
cd relay-ops-service
go test ./internal/accounthealth ./internal/notify ./internal/dailyreport ./internal/app -count=1
go test ./... -count=1
```

测试至少覆盖：旧失败 + 最新成功、旧 `balance_exhausted` + 最新成功、最新仍失败、数据过期、同时间稳定排序和恢复通知去重。

- [ ] **Step 3: 在线核对 Monitor 当前状态**

对 #118、#119 及每个活动分组读取最新探测时间、状态和错误码，确认最新成功时 Monitor 显示正常，历史失败只保留在历史记录中，不影响当前 Tier 和可用账号计数。

- [ ] **Step 4: 验证飞书出站配置与链接**

核对生产飞书 App、chat ID、recipient policy、notification policy 和投递审计。所有管理链接统一为 `/admin/accounts/monitor`；入站命令和写控制保持关闭。

- [ ] **Step 5: 增加 root-only 告警/恢复验收命令**

新增 `monitor-alert-acceptance`，只接受 `--run-id` 和 `--transition alert|recovery`。它复用现有 Feishu client、`notify.DeliverySender`、生产 notification repository 和现有告警/恢复 renderer；incident key 按 `acceptance:monitor-feishu:` 加实际 `run-id` 拼接，occurrence 固定为 1，账号名称固定为“飞书链路验收”，管理链接固定为 `/admin/accounts/monitor`。命令不得读取或修改真实账号、分组、优先级、路由、账单或探测表。

先写测试证明：alert 与 recovery 使用同一 incident key、不同 transition；相同 transition 重放只投递一次；非法 run ID 被拒绝；输出只包含 transition、delivery status 和 message ID，不包含飞书凭据。

- [ ] **Step 6: 执行不影响客户路由的告警/恢复验收**

```bash
ACCEPTANCE_RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
sudo docker exec sub2api-relay-ops-1 /monitor-alert-acceptance \
  --run-id "$ACCEPTANCE_RUN_ID" --transition alert
sudo docker exec sub2api-relay-ops-1 /monitor-alert-acceptance \
  --run-id "$ACCEPTANCE_RUN_ID" --transition recovery
sudo docker exec sub2api-relay-ops-1 /monitor-alert-acceptance \
  --run-id "$ACCEPTANCE_RUN_ID" --transition recovery
```

验证告警和恢复各投递一次，第三次 recovery 被去重，两个卡片链接均可打开账号监控页。

- [ ] **Step 7: 核对真实分类不会继承历史错误码**

只读查询最近成功账号对应的 incident/notification 状态，确认不存在活动 `balance_exhausted`、`no_available_account` 或其他由旧样本继承的事件；历史已恢复事件保留审计但不进入活动告警。

- [ ] **Step 8: 审查、记录并更新总账**

完成证据必须同时含：relay-ops 镜像身份、Monitor 在线当前状态、飞书告警 message ID、恢复 message ID、通知审计记录和无重复投递证明。

---

### Task 6: 发布 OpenAI 实际响应模型审计与管理端展示

**Files:**
- Review: `upstream/sub2api/backend/migrations/193_usage_log_actual_response_model.sql`
- Review: `upstream/sub2api/backend/ent/schema/usage_log.go`
- Review: `upstream/sub2api/backend/internal/service/openai_response_model_audit.go`
- Review: `upstream/sub2api/backend/internal/service/openai_gateway_*`
- Review: `upstream/sub2api/backend/internal/service/openai_ws_v2/`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers.go`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/UsageView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
- Modify: `docs/superpowers/plans/2026-08-02-openai-model-mapping-audit-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-08-05-openai-actual-model-production-verification.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: backend nullable字段 `usage_logs.actual_response_model`、best-effort response model extractor、admin usage DTO。
- Produces: 管理员用量日志中的“请求模型”和“实际响应模型”双字段；不改变客户端响应、路由、计费或调度。

- [ ] **Step 1: 对 canonical tree 做差异审计**

确认 backend 基础提交 `c53bbdf48`、`f20ab6a99`、`7518ac689` 已存在；不得把本地 `main` 或 `122d293db` 整体合入。用逐文件 diff 提取 canonical tree 缺少的 DTO、前端列和测试，优先移植 `45921782a` 的六个前端文件。

- [ ] **Step 2: 写/恢复失败测试**

测试覆盖：

- JSON 顶层和嵌套 `model`。
- SSE 只接受最终/完成事件，不被任意增量事件覆盖。
- Responses WebSocket 实际模型记录。
- 缺失或非法 model 保持 null。
- model 最长 100 字符。
- 审计写入失败不影响请求成功。
- 客户端收到的 JSON/SSE/WebSocket 字节或消息完全不变。
- 管理端表格分别显示请求模型与实际响应模型，null 显示 `—`。

- [ ] **Step 3: 移植最小缺失实现**

只补 canonical tree 缺少的 DTO mapper、TypeScript 类型、UsageTable 列、筛选/导出映射和中英文文案。不得回退账号监控 V3、迁移 194-196、会话隔离或其他后续修复。

- [ ] **Step 4: 运行完整目标测试**

```bash
cd upstream/sub2api/backend
go test ./migrations ./ent/schema ./internal/repository ./internal/service ./internal/handler/dto ./internal/handler/admin -count=1
cd ../frontend
npm run test:unit -- --run src/components/admin/usage/__tests__/UsageTable.spec.ts
npm run type-check
npm run build
```

- [ ] **Step 5: 独立审查安全边界**

审查必须证明：只持久化模型字符串，不保存原始响应；无新索引和无高频全表更新；best-effort 写入不阻断流式传输；OpenAI 之外协议不受影响；requested model 与 actual response model 不混用。

- [ ] **Step 6: 使用 Sub2API 蓝绿流程发布**

发布 API/worker 和前端静态资源，保持 PostgreSQL、Redis、Caddy 身份不变。迁移 193 必须为向前兼容 nullable `VARCHAR(100)`，失败时回滚应用镜像，不删除列。

- [ ] **Step 7: 线上协议验收**

使用现有受控测试 Key 分别发送一条 OpenAI-compatible JSON、SSE 和 Responses WebSocket 请求。每条请求记录 request ID，验证：客户端响应成功且未被改写；`usage_logs.model` 保留请求模型；`actual_response_model` 写入上游真实模型；管理员 Usage 页面显示两列。验收后禁用或删除临时测试 Key。

- [ ] **Step 8: 最终全分支审查与总账收口**

最终审查覆盖六个阶段的提交范围、生产证据、回滚点和文档一致性。只有六个阶段各自满足三项完成证据，才把六阶段总事项标记“已完成”；否则按具体阶段继续保持“进行中”。

## Plan Self-Review

### Spec coverage

- 六个方向均对应一个独立任务，并保持用户给出的执行顺序。
- 账号监控与营收职责已彻底分离。
- 总账、推送、部署、线上验证三项门槛贯穿每个阶段。
- 真实账单授权、全站映射、非零数据和日结阻断均有明确验收。
- Monitor/飞书和 OpenAI 审计均包含真实线上证据，不以本地测试代替完成。

### Placeholder scan

- 动态生产值只允许在执行报告中由只读预检确定，包括账本起始日、内部 ID 集合、镜像摘要和账号清单。
- 计划没有把示例日期、示例账号或未知凭据当成生产事实。

### Type and contract consistency

- Task 3 产出的真实账务数据是 Task 4 的唯一经营数据来源。
- Task 2 部署的 relay-ops 镜像同时承载 Task 5 已存在的最新状态修复。
- Task 6 只补 OpenAI 审计缺失面，不重新实现已在 canonical tree 中的后端基础。
