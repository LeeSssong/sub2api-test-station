# T33 经营页账号卡片与搜索实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将经营页 USD/CNY 账号表格改为响应式独立账号卡片并增加本地搜索，同时保持原生数据与采购链路。

**Architecture:** 继续在 `AccountProfitabilityView.vue` 内使用现有 report/selfPurchased 数据和生命周期；以两个卡片模板与共享搜索计算属性替换表格。仅修改直接页面测试，不增加后端接口或子系统。

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Tailwind utility classes, Vitest + Vue Test Utils.

**Spec:** `docs/superpowers/specs/2026-08-19-t33-profitability-account-cards-search-design.md`

## Global Constraints

- 复用 `accountFinancial` 与 `selfPurchasedProfitability`，不新建经营 API。
- USD 原生金额保持 USD；CNY 采购字段保持 CNY，额度/标准消耗标记 USD；不新增汇率、账务源、迁移或生产数据。
- 卡片桌面双列、390px/窄屏单列，页面不得整页横向溢出，长账号名必须换行。
- 保留范围、分组、排序、加载/错误/刷新/采购操作行为。

### Task 1: 先写卡片与搜索回归测试（RED）

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

- [ ] **Step 1: 增加 USD/CNY 卡片选择器与固定字段断言**：断言 `account-card-grid`、`account-card-<id>`、USD/CNY metric keys、桌面 `lg:grid-cols-2` 与移动 `grid-cols-1`。
- [ ] **Step 2: 增加本地搜索矩阵**：输入名称、ID、平台、类型、USD active/historical、CNY status/cost_status；断言卡片数量与 API 调用次数不变，清空恢复。
- [ ] **Step 3: 增加 390px/长名断言**：挂载 390px 容器，断言主页面没有横向溢出契约类名、卡片名有 `break-words`，CNY 也使用单列卡片。
- [ ] **Step 4: 运行 RED**：
  `cd upstream/sub2api/frontend && pnpm exec vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts`
  预期新增断言因当前表格/无搜索而失败。

### Task 2: 实现共享搜索与 USD 卡片（GREEN）

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountFinancial.ts` (only optional normalized status field if needed)

- [ ] **Step 1: 增加 `searchQuery`、搜索输入和 `filteredAccounts`**：统一 lowercase 文本匹配 name/id/platform/type/status，USD status fallback 为 `historical` 或 `active`；过滤后再执行既有排序。
- [ ] **Step 2: 用 `account-card-grid` 替换 USD 表格**：每账号独立 `article`，外层 `grid grid-cols-1 gap-4 lg:grid-cols-2`；账号头部与固定 `grid grid-cols-2` metric cells 展示 operational/business/revenue/total/net/margin。
- [ ] **Step 3: 保留 scope/sort/empty/loading/error/refresh 数据测试标识**：`account-row-<id>` 兼容为卡片根元素，`data-metric` 继续暴露字段。
- [ ] **Step 4: 运行直接测试并达到 GREEN**：同 Task 1 命令，修正模板/类型错误直到通过。

### Task 3: 实现 CNY 卡片与采购操作保留（GREEN）

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`

- [ ] **Step 1: 增加 CNY `filteredSelfPurchasedRows`**：同一 `searchQuery` 匹配 name/id/platform/account_type/status/cost_status，空查询保留全部。
- [ ] **Step 2: 用 `self-purchased-card-grid` 替换 CNY 表格**：每行一个卡片，固定两列字段网格；保留采购成本、预计额度 USD、标准消耗 USD、利用率、确认成本、待摊、采购损失、营收、净利润、利润率、成本状态、编辑/录入/确认失效按钮和现有 data-test。
- [ ] **Step 3: 保留 CNY 摘要、loading/error/retry/refresh 与共享 Dialog**；空搜索结果显示明确 empty 状态。
- [ ] **Step 4: 运行直接测试**：确认旧采购保存/结算测试仍通过，并修正 CNY 选择器。

### Task 4: 收口验证与候选交接

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts` (only test cleanup)
- Create: `docs/handoffs/2026-08-19-t33-profitability-account-cards-search-handoff.md`

- [ ] **Step 1: 运行直接 Vitest、typecheck、production build、git diff --check**。
- [ ] **Step 2: 做范围扫描**：确认没有后端、迁移、配置、生产状态、全局队列/进度文件差异。
- [ ] **Step 3: 检查候选状态与 SHA**：记录基线 `dc51b37c9dbf73a87cccceab5815f129882812c5`、候选提交 SHA、变更文件、测试、未验证项、迁移/配置、`downtime_required=false` 预期、回滚和风险。
- [ ] **Step 4: 提交候选并将状态报告为 `READY_FOR_ROOT_REVIEW`**；不合并、不推送、不部署。

## Self-review checklist

- [x] Spec requirements map to Tasks 1–4.
- [x] No new API/backend/financial source is introduced.
- [x] Both views have explicit search fields and responsive card constraints.
- [x] Existing procurement dialog and status/error lifecycle remain in scope.
