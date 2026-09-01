# T91-A 额度账务基础 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 T91-A 的源码核验、十进制定点适配边界、账务 schema/Ent、迁移合同和只读对账基础，为后续 T91-B～T91-E 提供可验证地基。

**Architecture:** 先以只读 Task 0 形成真实源码映射和 acceptance 基线；报告被发布总控接受后，再以 additive migration 增加 grant/adjustment 事实表及现有表字段。新账务域使用 `decimal.Decimal`，旧 Ent `float64` 只能经统一十进制适配器进入新域；本包不实现充值履约、API 扣费、退款 worker、管理员扣减、双写或 cutover。

**Tech Stack:** Go、Ent、PostgreSQL、现有迁移 runner、shopspring/decimal、现有单元/集成测试工具。

**Spec:** `/Users/gongtengxinwen/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_9944479444511_c3ab/msg/file/2026-09/2026-08-31-quota-accounting-rules-development-plan(1).md`

## Global Constraints

- Task 0 报告被接受前，不修改运行时代码、Ent schema、迁移、生产数据或验收站数据。
- 任务只交付 T91-A；T91-B～E 不得顺带实现。
- `attempted_quota_usd` 表示应扣额度；`delta_usd` 表示实际扣费；未扣差额不进余额公式且不形成用户欠款。
- 新账务金额使用 `decimal.Decimal` 与 `numeric(20,8)`；不得以 `float32/float64` 承担新账务计算。
- 退款不确定结果保持 `pending/unknown/reconciling`；dead-letter 不得自动转为明确失败或释放 reservation。
- 所有部署和 acceptance migration 只能从干净且与 `origin/main` 一致的根 `main` 发起；当前根 `main` 尚领先远端，禁止进入发布车道。

### Task 0: 源码映射与只读基线

**Files:**
- Create: `docs/superpowers/reports/2026-09-01-t91-a-source-mapping.md`
- Create: `docs/superpowers/notes/2026-09-01-t91-a-acceptance-baseline.md`
- Read-only inspect: `upstream/sub2api/backend/ent/schema/`, `internal/service/`, `internal/repository/`, `internal/handler/`, `migrations/`

- [ ] 记录仓库绝对路径、分支、commit、工作区状态和远端同步状态。
- [ ] 映射 payment order、wallet、users.balance、usage billing、redeem/promo、refund/provider、outbox、migration runner、Ent 生成命令和测试入口到实际文件/函数/路由。
- [ ] 建立余额不足矩阵基线：paid 足够、paid 不足 gift 足够、两者都不足、opening 负值、失败请求、零费用请求；记录响应、实际扣费和旧 ledger 行为。
- [ ] 建立重复支付回调、重复 API 请求、重复兑换和旧自动码的只读基线；不得写入生产事实。
- [ ] 记录旧 ledger 数量、wallet 漂移、负余额、外键/索引、provider 实例和交易号唯一范围；输出脱敏结果。
- [ ] 将报告提交发布总控审阅；未接受前停止后续写入步骤。

### Task 1: 十进制定点与 Ent 可行性合同

**Files:**
- Create: `docs/superpowers/decisions/2026-09-01-t91-a-decimal-boundary.md`
- Inspect: `upstream/sub2api/backend/ent/schema/payment_order.go`
- Inspect: `upstream/sub2api/backend/internal/payment/types.go`
- Inspect: `upstream/sub2api/backend/internal/service/payment_amounts.go`

- [ ] 列出旧 `float64` 边界和新账务 `decimal.Decimal` 边界。
- [ ] 确定新 `numeric(20,8)` 字段采用 Ent 自定义类型、原生 SQL Scanner/Valuer 或等价可编译方案。
- [ ] 定义字符串解析、8 位规范化、HALF_UP 兜底和币种转换错误合同。
- [ ] 写出后续 T91-B～D 必须复用的适配器接口和禁止事项；本任务不实现业务服务。

### Task 2: Additive schema 与迁移设计

**Files:**
- Create: `upstream/sub2api/backend/migrations/20260901_quota_accounting_foundation.sql`
- Modify after Task 0 acceptance: relevant files under `upstream/sub2api/backend/ent/schema/`
- Test: migration schema/constraint/rerun tests under `upstream/sub2api/backend/migrations/`

- [ ] 创建 `user_quota_grants` 与 `user_quota_adjustments` 的 additive schema。
- [ ] 为 payment_orders、payment_audit_logs、billing_usage_entries、redeem_codes、promo_codes、promo_code_usages 增加兼容新列。
- [ ] 添加部分唯一索引、FIFO 索引、来源外键、状态检查和 allocation JSON 约束。
- [ ] 保留旧表/旧列；循环外键按两阶段创建；不得修改已上线迁移文件。
- [ ] 覆盖迁移首次执行、重复执行、事务失败和 schema/constraint 验证。

### Task 3: 只读 reconciliation 基础

**Files:**
- Create: `upstream/sub2api/backend/cmd/quota-reconciliation/` or repository-conventional equivalent discovered in Task 0
- Test: direct reconciliation unit/integration tests

- [ ] 输出 wallet 公式差异、allocation 差异、订单退款汇总差异、重复幂等键、无效 JSON、跨用户 grant 和历史未知残差。
- [ ] 以用户和全局两级返回差异；任一差异返回非零退出码。
- [ ] 保存批次标识、查询时间、数据库版本和脱敏报告；不自动修余额、不回填历史事实。

### Task 4: T91-A 验证与交接

- [ ] 运行 T91-A 直接相关 migration schema/constraint/rerun/rollback 测试。
- [ ] 运行 `gofmt`、`git diff --check`、必要的 server/Ent 生成验证和范围检查。
- [ ] 自查 T91-B～E 未泄漏到 T91-A；确认未执行 acceptance migration、真实退款、历史回填、双写或 cutover。
- [ ] 形成 handoff，列出基线 SHA、变更文件、测试结果、未验证项、迁移变化、回滚方式和剩余风险。
- [ ] 将任务状态置为 `READY_FOR_ROOT_REVIEW`，等待根总控授权，不自行合并、推送或部署。
