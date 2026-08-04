# 账号监控卡片数据完善 V3 生产实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将已确认的 V3 账号卡片原型实施到 Sub2API 管理后台，补齐采购成本、真实请求窗口评分、原生分组汇总和实时并发，并从账号监控页移除营收与对账内容。

**Architecture:** 保留现有账号监控聚合入口，但为 `range=24h|7d|30d` 增加按 `usage_logs` 计算的窗口证据。采购成本存入账号原生表并复用 `expires_at`，实时并发通过独立批量端点读取 Redis，避免每 5 秒重算长期聚合。前端以已确认的 V3 原型为视觉真值，账号监控页只承担账号服务质量与调度信息。

**Tech Stack:** Go 1.24、Gin、Ent、PostgreSQL、Redis、Vue 3、TypeScript、Vitest、Tailwind CSS、pnpm 9。

## Global Constraints

- Any resumed controller, implementer, reviewer, or deployer must first read `docs/project/account-monitor-v3-acceptance-contract.md`; it overrides conflicting historical account-monitor decisions.
- The selected V3 screenshots and interactive prototype are the complete visual truth. Recent probes, selected-window call disclosure, and the checked-at/refresh footer are required card sections, not optional legacy content.
- 本轮不得部署、推送或修改生产服务器；最终停在部署前并列出待执行动作。
- `procurement_cost_cny` 可空且必须 `>= 0`；非空即采购模式，为空即倍率模式。
- `procurement_cost_effective_at` 首次录入或覆盖金额时由服务端写 UTC 当前时间；清空金额时同时清空生效时间。
- 采购有效期结束时间只使用原生 `expires_at`，金额与站内美元额度数值按 1:1，不做汇率换算。
- 窗口仅允许 `24h`、`7d`、`30d`，默认 `24h`；评分、排名、请求数、失败数、成功率、TTFT P50、总延迟 P95 和基础成本全部使用同一窗口。
- 真实请求数少于 3 时才允许同窗口探测补充服务质量证据；探测不得增加请求数、失败数或基础成本。
- 缺少有效采购期、基础成本或上游倍率时成本项为 0 分，但账号仍参与评分与排名。
- 排名按总分降序、账号 ID 升序稳定排序；唯一正常可用账号必须排名 1；未排名账号置后并按账号 ID 升序。
- 分组汇总只展示 `status`、`platform`、`rate_multiplier`、`rpm_limit`、`account_count`、`active_account_count`、`rate_limited_account_count`。
- 分组倍率只出现在分组汇总，不出现在账号卡片。
- 当前并发每 5 秒通过单次批量 Redis pipeline 更新；页面隐藏时停止，恢复时立即刷新；失败保留最后成功值并标记“数据延迟”。
- 全局优先级只允许整数 `>= 1`，服务端成功后才更新显示；失败保留原值、草稿、焦点和局部错误。
- 桌面端每个分组一行两张卡片，移动端一行一张，按组内排名渲染。
- `/admin/accounts/monitor` 不得保留营收、利润、账务、对账、历史流水、异常处理或运营汇总。
- 视觉实现以 `docs/prototypes/account-monitor-card-v2/index.html`、`prototype-v3-desktop.png` 和 `prototype-v3-mobile-top.png` 为真值。
- 所有行为改动遵循 RED-GREEN-REFACTOR；实现代理必须记录失败测试和成功测试的命令与输出。

---

### Task 1: 采购成本持久化与账号更新契约

**Files:**
- Create: `upstream/sub2api/backend/migrations/196_account_procurement_cost.sql`
- Modify: `upstream/sub2api/backend/ent/schema/account.go`
- Modify: `upstream/sub2api/backend/ent/account.go`
- Modify: `upstream/sub2api/backend/ent/account/where.go`
- Modify: `upstream/sub2api/backend/ent/account_create.go`
- Modify: `upstream/sub2api/backend/ent/account_update.go`
- Modify: `upstream/sub2api/backend/ent/account_update_one.go`
- Modify: `upstream/sub2api/backend/ent/mutation.go`
- Modify: `upstream/sub2api/backend/ent/runtime/runtime.go`
- Modify: `upstream/sub2api/backend/ent/schema.go`
- Modify: `upstream/sub2api/backend/internal/service/account.go`
- Modify: `upstream/sub2api/backend/internal/service/admin_service.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Test: `upstream/sub2api/backend/migrations/account_procurement_cost_migration_test.go`
- Test: `upstream/sub2api/backend/internal/service/account_procurement_cost_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_handler_test.go`

**Interfaces:**
- Produces: `Account.ProcurementCostCNY *float64` and `Account.ProcurementCostEffectiveAt *time.Time`.
- Produces: account update JSON fields `procurement_cost_cny` and read-only response field `procurement_cost_effective_at`.
- Produces: update semantics where an omitted field preserves both values, a number stores it and resets effective time to `time.Now().UTC()`, and explicit JSON `null` clears both.

- [ ] **Step 1: Write failing migration, service, and handler tests**

Add literal assertions that migration 196 creates nullable `numeric(14,2)` and `timestamptz` columns with a nonnegative check; that `12.50`, `0`, omitted, and explicit `null` produce the specified state transitions; and that negative, NaN, and infinite values return HTTP 400 without repository writes.

- [ ] **Step 2: Run focused tests and record RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./migrations ./internal/service ./internal/handler/admin -run 'ProcurementCost|AccountUpdate' -count=1
```

Expected: FAIL because the fields, migration, and update contract do not exist.

- [ ] **Step 3: Implement migration, Ent fields, domain mapping, and update semantics**

Use Ent nullable float/time fields, generate Ent code with:

```bash
cd upstream/sub2api/backend
go generate ./ent
```

Represent explicit-null distinctly from omitted input in the handler, validate finite `>= 0`, set effective time on every non-null amount write, and clear both fields on explicit null. Do not add a second expiry field or currency conversion.

- [ ] **Step 4: Run focused tests and record GREEN**

```bash
cd upstream/sub2api/backend
go test ./migrations ./internal/service ./internal/handler/admin -run 'ProcurementCost|AccountUpdate' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/migrations upstream/sub2api/backend/ent upstream/sub2api/backend/internal
git commit -m "feat(admin): persist account procurement cost"
```

### Task 2: 真实请求窗口评分、稳定排名与原生分组汇总

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`

**Interfaces:**
- Consumes: Task 1 account procurement fields and native `expires_at`.
- Produces: `GET /api/v1/admin/accounts/monitor?range=24h|7d|30d` with default `24h` and HTTP 400 for any other value.
- Produces: group fields `status`, `platform`, `rpm_limit`, `account_count`, `active_account_count`, `rate_limited_account_count` in addition to existing `rate_multiplier`.
- Produces account window fields: `range`, `request_count`, `error_count`, `base_cost`, `effective_multiplier`, `cost_mode`, `cost_score`, `quality_score`, `group_rank`, and evidence source/sample counts.

- [ ] **Step 1: Write failing service tests for cost and ranking**

Use fixed UTC times and literal expected values to cover procurement-window overlap, 1:1 effective multiplier, zero cost score for missing expiry/base cost/multiplier, continued rank eligibility, probe supplementation only below 3 real requests, unique healthy account rank 1, descending score ordering, account-ID tie break, and unranked ordering.

- [ ] **Step 2: Write failing repository and handler tests**

Require the repository SQL to aggregate `usage_logs` by account and selected `[since, until)` window with `COUNT(*)`, failed-request count, `SUM(total_cost)`, success rate, TTFT P50, and duration P95. Require group projection to read exactly the seven native fields and the handler to default/validate `range`.

- [ ] **Step 3: Run focused tests and record RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AccountMonitor.*(Window|Cost|Rank|Group|Range)|ListWindow' -count=1
```

Expected: FAIL because selected-window metrics and native group projection are absent.

- [ ] **Step 4: Implement selected-window aggregation and projection**

Add a typed range parser mapping `24h`, `7d`, `30d` to durations. Calculate procurement overlap as:

```text
max(0, min(window_end, expires_at) - max(window_start, effective_at))
purchase_cost * overlap / (expires_at - effective_at)
```

Use `usage_logs.total_cost` directly as base cost. Keep real request/error counts independent of probes. Supplement quality evidence from monitor probes only when real request count is less than 3. Apply existing weight normalization and cost-difference formula, then assign deterministic ranks after scores are complete.

- [ ] **Step 5: Run focused tests and record GREEN**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AccountMonitor.*(Window|Cost|Rank|Group|Range)|ListWindow' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_* upstream/sub2api/backend/internal/repository/account_monitor_repo* upstream/sub2api/backend/internal/handler/admin/account_monitor_handler*
git commit -m "feat(admin): score account monitor by request window"
```

### Task 3: 批量实时并发接口

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Test: `upstream/sub2api/backend/internal/server/routes/admin_test.go`

**Interfaces:**
- Consumes: `ConcurrencyService.GetAccountConcurrencyBatch(ctx, []int64) (map[int64]int, error)` and each account's native `concurrency` maximum.
- Produces: `POST /api/v1/admin/accounts/monitor/concurrency` body `{"account_ids":[1,2]}` and response `{"items":[{"account_id":1,"current":3,"limit":10}]}`.
- Rejects: empty input, nonpositive IDs, duplicate-normalized payloads exceeding 200 unique IDs, and accounts not visible to the authenticated admin request.

- [ ] **Step 1: Write failing handler and route tests**

Assert one service batch call for multiple IDs, stable response order matching first occurrence, deduplication, native maximum projection, HTTP 400 validation, HTTP 500 Redis failure, and authenticated route registration.

- [ ] **Step 2: Run focused tests and record RED**

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin ./internal/server/routes -run 'AccountMonitor.*Concurrency' -count=1
```

Expected: FAIL because the lightweight endpoint is absent.

- [ ] **Step 3: Implement the handler and route**

Inject the existing concurrency service into the monitor handler, resolve requested accounts in one repository operation, invoke exactly one `GetAccountConcurrencyBatch`, and never trigger monitor aggregation or per-card Redis calls.

- [ ] **Step 4: Run focused tests and record GREEN**

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin ./internal/server/routes -run 'AccountMonitor.*Concurrency' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal/handler upstream/sub2api/backend/internal/server/routes
git commit -m "feat(admin): expose monitor concurrency batch"
```

### Task 4: 前端 API 契约与 V3 账号卡片交互

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

**Interfaces:**
- Consumes: Tasks 1-3 monitor, account update, and concurrency response fields.
- Produces: `AccountMonitorRange = '24h' | '7d' | '30d'`, `list(range)`, `getConcurrency(accountIDs)`, and procurement-cost update typing.
- Produces card events: `updatePriority(accountID, priority)`, `updateProcurementCost(accountID, cost|null)`, with promise-backed success/failure completion controlled by the parent.

- [ ] **Step 1: Rewrite component tests first**

Require the card to render account name/ID, status at top right, independent score/rank/priority row, success rate, TTFT P50, latency P95, account cost, and current/maximum concurrency. Cover multiplier mode, procurement mode with native expiry/effective multiplier, missing-cost reason, priority integer validation, save success, save failure preserving draft/focus/error, cost save/clear confirmation, and delayed concurrency retaining the previous value. Assert group multiplier and all revenue/profit/reconciliation labels are absent.

- [ ] **Step 2: Run component tests and record RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: FAIL because the current card still uses operations data and lacks cost/concurrency interactions.

- [ ] **Step 3: Implement API types and card**

Match the approved prototype hierarchy and controls. Use the project's existing icon component for pencil, save, cancel, and clear controls; keep controls stable at mobile size; keep editable state until the awaited parent save resolves; use localized inline errors without reloading the page.

- [ ] **Step 4: Run component tests and record GREEN**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/frontend/src/api/admin upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.*
git commit -m "feat(admin): implement account monitor v3 card"
```

### Task 5: 账号监控页面收敛、窗口切换与并发轮询

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Delete if now unused: account-monitor revenue/reconciliation-only child components imported solely by `AccountMonitorView.vue`

**Interfaces:**
- Consumes: Task 4 typed APIs and card events.
- Produces: tab-style 24h/7d/30d selector with 24h default, group tab navigation, seven-field group summary, rank-sorted two-column cards, and one visibility-aware 5-second concurrency poller.

- [ ] **Step 1: Rewrite view tests first**

Require default `list('24h')`, each range switch reload, last-complete-snapshot retention on range error, group tabs, exactly seven native summary fields, rank then ID ordering, `lg:grid-cols-2` and mobile single-column layout, priority/cost saves followed by current-range reload, one deduplicated concurrency batch call, 5-second polling, pause on `document.hidden`, immediate resume refresh, and failure retention with delayed flags. Assert reconciliation APIs are never invoked and monitor DOM contains no revenue, profit, operations, ledger, history, reconciliation, adjustment, or exception sections.

- [ ] **Step 2: Run view tests and record RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected: FAIL because the current view contains operations/reconciliation sections and lacks selected-window/concurrency orchestration.

- [ ] **Step 3: Implement the V3 page**

Remove all operations/reconciliation imports, state, API calls, dialogs, summaries, and card props. Render the approved tab hierarchy and group summary. Deduplicate accounts by ID, sort ranked accounts by rank then ID, sort unranked accounts after them by ID, poll only unique visible card IDs, and preserve the last successful concurrency number on failures.

- [ ] **Step 4: Run focused frontend tests and record GREEN**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

Expected: PASS.

- [ ] **Step 5: Run frontend type and build checks**

```bash
cd upstream/sub2api/frontend
pnpm type-check
pnpm build
```

Expected: both commands exit 0.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/frontend/src/views/admin upstream/sub2api/frontend/src/components/admin/account-monitor upstream/sub2api/frontend/src/api/admin
git commit -m "feat(admin): ship focused account monitor v3"
```

### Task 6: 账号监控页面按已确认设计稿 1:1 还原

**Visual truth:**
- `/var/folders/26/3qc7y_lx2s11df_9sh7dqg_40000gn/T/codex-clipboard-d94eb487-1975-42a0-9619-6c96a939c5b9.png`
- `docs/prototypes/account-monitor-card-v2/prototype-v3-desktop.png`
- `docs/prototypes/account-monitor-card-v2/prototype-v3-mobile-top.png`

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Preserve Task 4/5 priority, procurement-cost, selected-window, ranking, group-summary, and concurrency behavior.
- Restore the service-only lower card modules removed by `2ea3ef167`: recent-probe bars from the last 24 `timeline` points, selected-window call disclosure, checked-at/statistics-cutoff footer, and per-card refresh.
- Match the screenshot hierarchy: constrained page width; search + one status selector + range control; enabled group tab and seven-field summary; normal cards with green left border and pale-green header; three-cell score/rank/priority band; five distinct metric tiles; recent-probe section; call disclosure; footer.
- The filter row must not render the extra platform selector shown by the rejected implementation.
- `24h`, `7d`, and `30d` render `24 小时调用`, `7 天调用`, and `30 天调用` from the same selected account window.
- Probe summaries and failure bars come only from `timeline`; real request/error counts remain the call disclosure and never include probes.
- Keep the page free of revenue, profit, operations, accounting, ledger, history, reconciliation, adjustment, and exception UI.
- At 1440x1000 render exactly two rank-sorted cards in one row with no overflow; at 390x844 render one column with no horizontal overflow.

- [ ] **Step 1: Write failing component and view tests**

Add real DOM assertions for the green card shell/header, five colored metric tiles, 24 probe bars and summary, selected-window call disclosure, checked-at footer, per-card refresh, single status selector, absence of a platform selector, constrained two-column layout, and retained no-revenue/no-reconciliation boundary.

- [ ] **Step 2: Run focused tests and record RED**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected: FAIL because the rejected implementation lacks the lower card modules and renders an extra platform selector.

- [ ] **Step 3: Implement the screenshot-faithful page**

Reuse the existing project `Icon` component and the earlier service-only probe/call/footer logic. Do not restore economics, reconciliation, settings, history, or operations props/events. Keep editable controls promise-backed and responsive.

- [ ] **Step 4: Run focused tests, typecheck, and production build**

```bash
cd upstream/sub2api/frontend
pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/admin/account-monitor/AccountMonitorFilters.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm typecheck
pnpm build
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/frontend/src/components/admin/account-monitor upstream/sub2api/frontend/src/views/admin
git commit -m "fix(admin): match account monitor v3 design"
```

## Final Verification Before Deployment

- Run backend focused suites, then `go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes ./migrations`, `go vet ./...`, and `go build ./...`.
- Run frontend focused suites, `pnpm type-check`, and `pnpm build`.
- Start the local production Vue page and compare at `1440x1000` and `390x844` against the approved V3 screenshots.
- Save desktop/mobile evidence under `docs/prototypes/account-monitor-card-v2/`, update `design-qa.md`, and require `final result: passed`.
- Run an independent whole-branch review and resolve all Critical/Important findings.
- Keep `docs/project/project-progress.md` as “进行中”, do not push or deploy, and report the exact next deployment sequence plus rollback commit `d4fb5e4a4`.
