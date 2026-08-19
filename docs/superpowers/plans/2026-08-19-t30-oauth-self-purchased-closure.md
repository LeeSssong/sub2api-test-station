# T30 OAuth 自购闭环 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让 CNY 自购报表覆盖全部未删除 OAuth 账号，并在每行复用账号监控采购成本弹窗完成保存、清空、刷新和可诊断错误闭环。

**Architecture:** 以 accounts OAuth 集合作为报表驱动表，LEFT JOIN 采购版本与 usage_logs；无配置账号生成 cost_pending 投影。CNY 页面复用 AccountMonitorCostDialog 和 accounts.updateProcurementCost，成功后刷新两处数据。

**Tech Stack:** Go、Gin、database/sql、PostgreSQL、Vue 3、TypeScript、Vitest、pnpm。

**Spec:** `docs/superpowers/specs/2026-08-19-t30-oauth-self-purchased-closure-design.md`

## Global Constraints

- 不改迁移、配置、生产、全局队列/总账或 GitHub Actions。
- 仅复用 accounts.procurement_cost_cny、accounts.estimated_usable_quota_usd、account_procurement_cost_versions 和既有 PUT。
- 候选最终状态为 `READY_FOR_ROOT_REVIEW`。

### Task 1: 报表候选集改为全量 OAuth

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_procurement_profitability.go`
- Test: `upstream/sub2api/backend/internal/service/account_profitability_test.go`

- [x] 写失败测试：无版本/无采购字段 OAuth 账号仍返回一行，成本状态 `cost_pending`，流水字段为 0；非 OAuth/已删除账号排除；现有采购版本账号结果不变。
- [x] 运行 `go test ./internal/service -run 'SelfPurchasedReport' -count=1`，确认新增用例先失败。
- [x] 将 SQL 的 `versions`/`scoped` 改为 accounts OAuth 驱动的 LEFT JOIN，保留版本时间窗口和 usage 过滤；为无版本行返回 synthetic version 0 与空成本。
- [x] 在 Go 聚合中识别无版本/部分 legacy projection，生成 `cost_pending` 且不丢失实际流水。
- [x] 运行同一测试并确认通过，再执行相关 service 测试。

### Task 2: 修复 internal error 真实失败链路与保存后刷新语义

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/dashboard_handler.go`
- Modify: `upstream/sub2api/backend/internal/service/account_procurement_cost_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_handler_procurement_cost_test.go`

- [x] 写失败测试复现截图1的 internal error 路径，覆盖真实事务/handler 错误映射、成功响应投影和重复 request id replay。
- [x] 运行定向 Go 测试确认 RED。
- [x] 最小修复真实失败根因与错误映射；保存成功响应包含最新采购投影，前端后续刷新失败不得覆盖保存成功。
- [x] 运行 handler/service 定向测试与 gofmt。

### Task 3: CNY 表逐行共享成本入口

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountProfitabilityView.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/selfPurchasedProfitability.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountProfitabilityView.spec.ts`

- [x] 写 Vitest RED：每行有录入/编辑按钮；无成本默认额度 60；点击打开共享 `AccountMonitorCostDialog`；保存/清空调用 `adminAPI.accounts.updateProcurementCost`，成功后刷新 CNY 与账号监控数据。
- [x] 运行 `pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts` 确认 RED。
- [x] 在 CNY 视图挂载共享弹窗，构造最小 `AccountMonitorAccount` 适配器，复用既有会话幂等与 API；增加行级按钮、保存/清空回调及刷新协调。
- [x] 更新 API 类型以承载 `procurement_readback_status`/稳定错误码及 interceptor partial-success 归一化。
- [x] 运行 Vitest，确认绿灯。

### Task 4: 直接相关验证与交接

**Files:**
- Create: `docs/handoffs/2026-08-19-t30-oauth-self-purchased-closure-handoff.md`

- [x] 运行 Go 定向测试、Vitest、typecheck、build、diff-check。
- [x] 执行 DOM 合同检查；记录未执行的认证浏览器截图项。
- [x] 写 handoff：基线 SHA、候选 SHA、变更文件、测试、未验证项、迁移/配置、downtime_required、回滚、剩余风险；状态标记 `READY_FOR_ROOT_REVIEW`。
